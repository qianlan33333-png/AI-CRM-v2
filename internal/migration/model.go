// Package migration contains the application core for bounded legacy-data imports.
// It deliberately owns no database driver, source credentials, or domain tables.
package migration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrInvalidManifest       = errors.New("invalid migration manifest")
	ErrUnknownAdapter        = errors.New("unknown migration adapter")
	ErrUnknownPolicy         = errors.New("unknown migration policy")
	ErrSourceDrift           = errors.New("migration source identity or schema drift")
	ErrUnboundedStream       = errors.New("migration source stream is not bounded")
	ErrSourcePayloadConflict = errors.New("migration source key payload conflict")
	ErrLeaseFenced           = errors.New("migration lease fenced")
	ErrTargetTampered        = errors.New("migration target tampered")
	ErrInvalidRun            = errors.New("invalid migration run")
)

type Digest [sha256.Size]byte

func Sum(value []byte) Digest { return sha256.Sum256(value) }

type Family string

const (
	FamilyContact     Family = "contact"
	FamilyIdentity    Family = "identity"
	FamilyWeCom       Family = "wecom"
	FamilyCampaign    Family = "campaign"
	FamilyOutbound    Family = "outbound"
	FamilyAutomation  Family = "automation"
	FamilySurvey      Family = "survey"
	FamilyMedia       Family = "media"
	FamilyRadar       Family = "radar"
	FamilyCommerce    Family = "commerce"
	FamilyPayment     Family = "payment"
	FamilyEntitlement Family = "entitlement"
	FamilyProduct     Family = "product"
	FamilyOperations  Family = "operations"
	FamilyPlatform    Family = "platform"
)

func (value Family) Valid() bool {
	switch value {
	case FamilyContact, FamilyIdentity, FamilyWeCom,
		FamilyCampaign, FamilyOutbound, FamilyAutomation,
		FamilySurvey, FamilyMedia, FamilyRadar,
		FamilyCommerce, FamilyPayment, FamilyEntitlement,
		FamilyProduct, FamilyOperations, FamilyPlatform:
		return true
	default:
		return false
	}
}

type StreamMode string

const (
	ModeSnapshot    StreamMode = "snapshot"
	ModeIncremental StreamMode = "incremental"
)

func (value StreamMode) Valid() bool { return value == ModeSnapshot || value == ModeIncremental }

type Disposition string

const (
	DispositionImport     Disposition = "import"
	DispositionArchive    Disposition = "archive"
	DispositionQuarantine Disposition = "quarantine"
	DispositionSkip       Disposition = "skip"
	DispositionRebuild    Disposition = "rebuild"
	DispositionReset      Disposition = "reset"
)

func (value Disposition) Valid() bool {
	switch value {
	case DispositionImport, DispositionArchive, DispositionQuarantine, DispositionSkip, DispositionRebuild, DispositionReset:
		return true
	default:
		return false
	}
}

type AdapterID string
type TableID string
type PolicyID string
type RunID string
type Cursor string

type TableSpec struct {
	ID             TableID
	SourceIdentity string // registry-owned source table/view identity; never caller supplied.
	SchemaDigest   Digest
	MappingDigest  Digest
	PolicyDigest   Digest
	PrimaryKey     string
	Watermark      string
	Mode           StreamMode
	Policy         PolicyID
}

func (spec TableSpec) valid() bool {
	return spec.ID != "" && spec.SourceIdentity != "" && spec.SchemaDigest != (Digest{}) && spec.MappingDigest != (Digest{}) && spec.PolicyDigest != (Digest{}) && spec.PrimaryKey != "" && spec.Watermark != "" && spec.Mode.Valid() && spec.Policy != ""
}

type AdapterManifest struct {
	ID                 AdapterID
	Family             Family
	SourceIdentity     string // opaque allowlisted identity, never a DSN.
	SourceSchemaDigest Digest
	Tables             []TableSpec
}

func (manifest AdapterManifest) Validate() error {
	if manifest.ID == "" || !manifest.Family.Valid() || manifest.SourceIdentity == "" || manifest.SourceSchemaDigest == (Digest{}) || len(manifest.Tables) == 0 {
		return ErrInvalidManifest
	}
	seen := make(map[TableID]struct{}, len(manifest.Tables))
	for _, table := range manifest.Tables {
		if !table.valid() {
			return ErrInvalidManifest
		}
		if _, exists := seen[table.ID]; exists {
			return ErrInvalidManifest
		}
		seen[table.ID] = struct{}{}
	}
	return nil
}

func (manifest AdapterManifest) Digest() (Digest, error) {
	if err := manifest.Validate(); err != nil {
		return Digest{}, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Digest{}, fmt.Errorf("marshal migration manifest: %w", err)
	}
	return Sum(encoded), nil
}

type Policy struct {
	ID          PolicyID
	Disposition Disposition
}

func (policy Policy) valid() bool { return policy.ID != "" && policy.Disposition.Valid() }

func (policy Policy) Digest() (Digest, error) {
	if !policy.valid() {
		return Digest{}, ErrUnknownPolicy
	}
	return Sum([]byte(string(policy.ID) + "\x00" + string(policy.Disposition))), nil
}

type UpperBound struct {
	Value []byte
	Empty bool
}

func (bound UpperBound) valid() bool {
	return (bound.Empty && len(bound.Value) == 0) || (!bound.Empty && len(bound.Value) > 0)
}

type TableBound struct {
	Table          TableID
	SourceIdentity string
	SchemaDigest   Digest
	UpperBound
}

type SourcePreflight struct {
	Identity     string
	SchemaDigest Digest
	Bounds       []TableBound
}

type SourceRow struct {
	Cursor      Cursor
	SourceKey   []byte
	Payload     []byte
	FieldDigest Digest // adapter-derived canonical field digest; raw fields never enter receipts.
}

func (row SourceRow) valid() bool {
	return row.Cursor != "" && len(row.SourceKey) > 0 && row.FieldDigest != (Digest{})
}

type StreamRequest struct {
	Table      TableSpec
	UpperBound UpperBound
	After      Cursor
	Limit      int
}

type StreamResult struct{ Complete bool }

// CursorCodec is supplied by the allowlisted adapter. The harness never guesses
// ordering for opaque source cursors.
type CursorCodec interface {
	Compare(Cursor, Cursor) (int, error)
}

type SourceAdapter interface {
	Preflight(context.Context) (SourcePreflight, error)
	Stream(context.Context, StreamRequest, func(SourceRow) error) (StreamResult, error)
}

type Mapper interface {
	MappingDigest(TableID) Digest
	Map(context.Context, TableSpec, SourceRow) (MappedRow, error)
}

type MappedRow struct {
	Operation string // adapter-owned named operation, never arbitrary SQL.
	Payload   []byte
	Digest    Digest
}

type AdapterDefinition struct {
	Manifest AdapterManifest
	Source   SourceAdapter
	Mapper   Mapper
	Cursors  CursorCodec
}

type MappingRegistry interface {
	Lookup(AdapterID) (AdapterDefinition, bool)
}

type PolicyRegistry interface {
	Lookup(PolicyID) (Policy, bool)
}
