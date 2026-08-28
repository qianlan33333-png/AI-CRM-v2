package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	referencehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contactreferencehistory"
	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// contactReferenceResolver reads only sealed lineage already owned by Contact
// and Identity. It never creates a Customer, Staff, identity, or assurance.
type contactReferenceResolver struct {
	archiveRun     string
	dm01Run        int64
	dm01KeyVersion int16
	archiveKey     []byte
	dm01Key        []byte
	corpID         string
	maps           map[string][]contactReferenceIdentityMap

	contacts      contactport.HistoricalImportTarget
	identities    identityport.HistoricalScopedWeComIdentityEvidenceReader
	staff         contactport.HistoricalImportStaffReader
	peopleJournal contactReferenceTerminalLoader
	deferred      contactReferenceDeferredReaderFactory
}

var _ v1domain.ContactReferenceResolver = (*contactReferenceResolver)(nil)

type contactReferenceIdentityMap struct {
	sourceID       int64
	sourceKey      [sha256.Size]byte
	payload        [sha256.Size]byte
	field          [sha256.Size]byte
	corpID         string
	externalUserID string
}

type contactReferenceTerminalLoader interface {
	LoadTerminal(context.Context, string) (v1domain.TerminalReceipt, bool, error)
}

type contactReferenceDeferredReader interface {
	GetHistoricalDeferredPerson(context.Context, int64) (contactport.HistoricalDeferredPerson, error)
}

type contactReferenceDeferredReaderFactory func(context.Context) (contactReferenceDeferredReader, error)

// newContactReferenceResolver pre-authenticates the complete source map once.
// Resolve methods only use the caller's existing UnitOfWork transaction.
func newContactReferenceResolver(ctx context.Context, uow *platformstore.UnitOfWork, archive referencehistory.ArchiveSource, archiveRun string, dm01Run int64, archiveKey, dm01Key []byte, corpID string) (*contactReferenceResolver, error) {
	if ctx == nil || uow == nil || archive == nil || archiveRun == "" || dm01Run < 1 || len(archiveKey) < sha256.Size || len(dm01Key) < sha256.Size || !contactReferenceCanonicalText(corpID) {
		return nil, v1domain.ErrInvalidScope
	}
	var expectedMaps int64
	if err := uow.Within(ctx, func(bound context.Context) error {
		var err error
		expectedMaps, err = v1domain.ReadContactReferenceMapCount(bound, archiveRun)
		return err
	}); err != nil {
		return nil, err
	}
	dm01, err := (v1domain.DeferredIdentityDM01Reader{UOW: uow}).ReadDM01Run(ctx, dm01Run)
	if err != nil {
		return nil, err
	}
	if dm01.ID != dm01Run || dm01.Mode != "full" || dm01.State != "imported" || dm01.HMACKeyVersion < 1 {
		return nil, v1domain.ErrConflict
	}
	maps, err := loadContactReferenceIdentityMaps(ctx, archive, archiveRun, archiveKey, expectedMaps)
	if err != nil {
		return nil, err
	}
	peopleJournal, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: v1domain.DeferredIdentityHistoryImportVersion,
		ArchiveRunID:  archiveRun,
		AdapterID:     v1archive.DefaultAdapterID,
		TableID:       v1deferredidentityhistory.PeopleTableID,
		TargetDomain:  v1domain.DeferredIdentityHistoryDomain,
		TargetTable:   v1domain.DeferredPersonHistoryTarget,
	})
	if err != nil {
		return nil, err
	}
	return &contactReferenceResolver{
		archiveRun: archiveRun, dm01Run: dm01Run, dm01KeyVersion: dm01.HMACKeyVersion,
		archiveKey: append([]byte(nil), archiveKey...), dm01Key: append([]byte(nil), dm01Key...), corpID: corpID, maps: maps,
		contacts: contactstore.HistoricalImportRepository{}, identities: identitystore.NewRepository(), staff: contactstore.NewStaffDirectoryRepository(nil), peopleJournal: peopleJournal,
		deferred: func(bound context.Context) (contactReferenceDeferredReader, error) {
			tx, err := platformstore.TxFromContext(bound)
			if err != nil {
				return nil, err
			}
			return contactstore.NewDeferredIdentityHistoryReader(tx), nil
		},
	}, nil
}

// ResolveBinding keeps an unavailable source relation NULL. A claimed but
// inconsistent DM01/identity record is a conflict, not a fallback.
func (r *contactReferenceResolver) ResolveBinding(ctx context.Context, value referencehistory.ExternalContactBindingFact) (v1domain.ContactBindingReferences, error) {
	if !r.valid() || ctx == nil || value.Source.SourceKeyDigest == ([sha256.Size]byte{}) || value.Source.PayloadDigest == ([sha256.Size]byte{}) || value.Source.FieldDigest == ([sha256.Size]byte{}) {
		return v1domain.ContactBindingReferences{}, v1domain.ErrInvalidScope
	}
	personID, err := r.resolvePerson(ctx, value.PersonID)
	if err != nil {
		return v1domain.ContactBindingReferences{}, err
	}
	identityID, assurance, err := r.resolveIdentity(ctx, value.ExternalUserID)
	if err != nil {
		return v1domain.ContactBindingReferences{}, err
	}
	return v1domain.ContactBindingReferences{PersonHistoryID: personID, IdentityID: identityID, IdentityAssurance: assurance}, nil
}

func (r *contactReferenceResolver) ResolveDirectory(ctx context.Context, value referencehistory.DirectoryMemberFact) (v1domain.ContactDirectoryReferences, error) {
	if !r.valid() || ctx == nil || value.Source.SourceKeyDigest == ([sha256.Size]byte{}) || value.Source.PayloadDigest == ([sha256.Size]byte{}) || value.Source.FieldDigest == ([sha256.Size]byte{}) {
		return v1domain.ContactDirectoryReferences{}, v1domain.ErrInvalidScope
	}
	if value.CorpID != r.corpID || value.WeComCorpID != r.corpID || !contactReferenceCanonicalText(value.WeComUserID) {
		return v1domain.ContactDirectoryReferences{CorpAttribution: "unattributable"}, nil
	}
	staff, err := r.staff.LockUniqueActiveStaffForHistoricalImport(ctx, value.WeComUserID)
	if errors.Is(err, contactport.ErrStaffReferenceNotFound) {
		return v1domain.ContactDirectoryReferences{CorpAttribution: "matched"}, nil
	}
	if err != nil || staff.ID < 1 {
		return v1domain.ContactDirectoryReferences{}, v1domain.ErrConflict
	}
	return v1domain.ContactDirectoryReferences{MatchedStaffID: &staff.ID, CorpAttribution: "matched"}, nil
}

func (r *contactReferenceResolver) resolvePerson(ctx context.Context, sourceID int64) (*int64, error) {
	sourceKey, err := v1archive.SourceKeyHMAC(r.archiveKey, "people", []byte("["+strconv.FormatInt(sourceID, 10)+"]"))
	if err != nil {
		return nil, v1domain.ErrConflict
	}
	terminal, found, err := r.peopleJournal.LoadTerminal(ctx, v1domain.SourceIdentifier(sourceKey))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	targetID, err := contactReferenceImportedTarget(terminal, sourceKey)
	if err != nil {
		return nil, err
	}
	reader, err := r.deferred(ctx)
	if err != nil {
		return nil, err
	}
	actual, err := reader.GetHistoricalDeferredPerson(ctx, targetID)
	if err != nil || actual.ID != targetID || actual.SourceID != sourceID || actual.SourceKeyDigest != sourceKey || actual.SourcePayloadDigest != terminal.PayloadDigest {
		return nil, v1domain.ErrConflict
	}
	digest, err := contactapp.HistoricalDeferredPersonDigest(actual)
	if err != nil || digest != terminal.TargetDigest {
		return nil, v1domain.ErrConflict
	}
	return &targetID, nil
}

func (r *contactReferenceResolver) resolveIdentity(ctx context.Context, externalUserID string) (*int64, string, error) {
	if !contactReferenceCanonicalText(externalUserID) {
		return nil, "unresolved", nil
	}
	candidates := r.maps[externalUserID]
	if len(candidates) == 0 {
		return nil, "unresolved", nil
	}
	for _, candidate := range candidates {
		if candidate.corpID != r.corpID {
			return nil, "unresolved", nil
		}
	}
	type resolvedIdentity struct {
		id        int64
		assurance string
	}
	resolved := map[resolvedIdentity]bool{}
	for _, candidate := range candidates {
		identity, found, err := r.resolveIdentityMap(ctx, candidate)
		if err != nil {
			return nil, "", err
		}
		if found {
			resolved[identity] = true
		}
	}
	if len(resolved) != 1 {
		return nil, "unresolved", nil
	}
	for value := range resolved {
		id := value.id
		return &id, value.assurance, nil
	}
	return nil, "unresolved", nil
}

func (r *contactReferenceResolver) resolveIdentityMap(ctx context.Context, value contactReferenceIdentityMap) (struct {
	id        int64
	assurance string
}, bool, error) {
	var empty struct {
		id        int64
		assurance string
	}
	dm01Source, err := contactmigration.SourceKeyHMAC(r.dm01Key, v1deferredidentityhistory.DM01IdentityMapSourceTable, strconv.FormatInt(value.sourceID, 10))
	if err != nil || len(dm01Source) != sha256.Size {
		return empty, false, v1domain.ErrConflict
	}
	if err = r.contacts.LockHistoricalImportSource(ctx, contactport.HistoricalImportExternalIdentity, dm01Source); err != nil {
		return empty, false, err
	}
	lineage, lineageFound, err := r.contacts.LockHistoricalImportLineage(ctx, contactport.HistoricalImportExternalIdentity, dm01Source)
	if err != nil {
		return empty, false, err
	}
	receipt, receiptFound, err := r.contacts.FindHistoricalImportRowReceipt(ctx, r.dm01Run, contactport.HistoricalImportExternalIdentity, dm01Source)
	if err != nil {
		return empty, false, err
	}
	if !lineageFound {
		if !receiptFound {
			return empty, false, nil
		}
		if receipt.Disposition == contactport.HistoricalImportQuarantined && len(receipt.PayloadHMAC) == sha256.Size && len(receipt.FieldDigest) == sha256.Size {
			return empty, false, nil
		}
		return empty, false, v1domain.ErrConflict
	}
	if !receiptFound || lineage.TargetID < 1 || lineage.LastRunID != r.dm01Run || len(lineage.PayloadHMAC) != sha256.Size || len(lineage.FieldDigest) != sha256.Size ||
		receipt.Disposition != contactport.HistoricalImportImported || !hmac.Equal(lineage.PayloadHMAC, receipt.PayloadHMAC) || !hmac.Equal(lineage.FieldDigest, receipt.FieldDigest) {
		return empty, false, v1domain.ErrConflict
	}
	evidence, err := r.identities.LockHistoricalScopedWeComIdentityEvidence(ctx, lineage.TargetID, dm01Source)
	if err != nil || evidence.IdentityID != lineage.TargetID || evidence.Scope != "wecom-corp:"+value.corpID || evidence.ExternalUserID != value.externalUserID || evidence.HMACKeyVersion != r.dm01KeyVersion ||
		(evidence.Assurance != identityport.AssuranceDeclared && evidence.Assurance != identityport.AssuranceVerified) {
		return empty, false, v1domain.ErrConflict
	}
	return struct {
		id        int64
		assurance string
	}{id: lineage.TargetID, assurance: string(evidence.Assurance)}, true, nil
}

func (r *contactReferenceResolver) valid() bool {
	return r != nil && r.archiveRun != "" && r.dm01Run > 0 && r.dm01KeyVersion > 0 && len(r.archiveKey) >= sha256.Size && len(r.dm01Key) >= sha256.Size && contactReferenceCanonicalText(r.corpID) && r.contacts != nil && r.identities != nil && r.staff != nil && r.peopleJournal != nil && r.deferred != nil
}

func contactReferenceImportedTarget(value v1domain.TerminalReceipt, sourceKey [sha256.Size]byte) (int64, error) {
	id, err := strconv.ParseInt(value.TargetID, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value.TargetID || value.SourceKeyDigest != sourceKey || value.PayloadDigest == ([sha256.Size]byte{}) || value.TargetDigest == ([sha256.Size]byte{}) || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 {
		return 0, v1domain.ErrConflict
	}
	return id, nil
}

func loadContactReferenceIdentityMaps(ctx context.Context, archive referencehistory.ArchiveSource, run string, key []byte, expected int64) (map[string][]contactReferenceIdentityMap, error) {
	if ctx == nil || archive == nil || run == "" || len(key) < sha256.Size || expected < 0 {
		return nil, v1domain.ErrInvalidScope
	}
	result := make(map[string][]contactReferenceIdentityMap)
	seen := make(map[[sha256.Size]byte]bool, expected)
	ordinal := int64(1)
	err := archive.EachTableRow(ctx, run, v1deferredidentityhistory.ExternalContactIdentityMapID, func(row v1archive.ArchivedRow) error {
		if row.SourceOrdinal != ordinal {
			return v1domain.ErrConflict
		}
		ordinal++
		fact, err := v1deferredidentityhistory.AdaptMissingRootIdentity(row, key)
		if err != nil {
			return v1domain.ErrConflict
		}
		var decoded struct {
			ID             int64  `json:"id"`
			CorpID         string `json:"corp_id"`
			ExternalUserID string `json:"external_userid"`
		}
		if json.Unmarshal(row.Payload, &decoded) != nil || decoded.ID != fact.SourceID || fact.Source.SourceKeyDigest == ([sha256.Size]byte{}) || fact.Source.PayloadDigest == ([sha256.Size]byte{}) || fact.Source.FieldDigest == ([sha256.Size]byte{}) ||
			[sha256.Size]byte(fact.Source.SourceKeyDigest) != row.SourceKeyHMAC || [sha256.Size]byte(fact.Source.PayloadDigest) != row.PayloadHMAC || [sha256.Size]byte(fact.Source.FieldDigest) != row.FieldHMAC || seen[[sha256.Size]byte(fact.Source.SourceKeyDigest)] {
			return v1domain.ErrConflict
		}
		seen[[sha256.Size]byte(fact.Source.SourceKeyDigest)] = true
		if !contactReferenceCanonicalText(decoded.CorpID) || !contactReferenceCanonicalText(decoded.ExternalUserID) || contactReferenceRedactedRoot(fact.RedactedRoots, "corp_id") || contactReferenceRedactedRoot(fact.RedactedRoots, "external_userid") {
			return nil
		}
		result[decoded.ExternalUserID] = append(result[decoded.ExternalUserID], contactReferenceIdentityMap{sourceID: decoded.ID, sourceKey: [sha256.Size]byte(fact.Source.SourceKeyDigest), payload: [sha256.Size]byte(fact.Source.PayloadDigest), field: [sha256.Size]byte(fact.Source.FieldDigest), corpID: decoded.CorpID, externalUserID: decoded.ExternalUserID})
		return nil
	})
	if err != nil || ordinal-1 != expected || int64(len(seen)) != expected {
		return nil, v1domain.ErrConflict
	}
	return result, nil
}

func contactReferenceCanonicalText(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && strings.TrimSpace(value) == value
}

func contactReferenceRedactedRoot(roots []string, name string) bool {
	for _, root := range roots {
		if root == name {
			return true
		}
	}
	return false
}
