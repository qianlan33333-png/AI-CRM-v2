// Package v1domain records V2-owned, immutable provenance for safe canonical
// imports from the encrypted V1 archive. It never stores source payloads.
package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var (
	ErrInvalidScope = errors.New("invalid V1 domain import scope")
	ErrConflict     = errors.New("V1 domain import receipt conflict")
)

type Scope struct {
	ImportVersion string
	ArchiveRunID  string
	AdapterID     string
	TableID       string
	TargetDomain  string
	TargetTable   string
}

type Journal struct {
	scope Scope
	tx    func(context.Context) (pgx.Tx, error)
}

type TerminalReceipt struct {
	SourceKeyDigest [sha256.Size]byte
	PayloadDigest   [sha256.Size]byte
	Disposition     string
	Reason          string
	TargetID        string
	TargetDigest    [sha256.Size]byte
	Metadata        map[string]any
}

var _ campaign.HistoricalDefinitionJournal = (*Journal)(nil)

func NewJournal(scope Scope) (*Journal, error) {
	if !scope.valid() {
		return nil, ErrInvalidScope
	}
	return &Journal{scope: scope, tx: platformstore.TxFromContext}, nil
}

func (scope Scope) valid() bool {
	return validVersion(scope.ImportVersion) && validToken(scope.ArchiveRunID, 128) &&
		validToken(scope.AdapterID, 128) && validTableID(scope.TableID) &&
		validToken(scope.TargetDomain, 128) && validIdentifier(scope.TargetTable, 63)
}

func validVersion(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func (journal *Journal) LoadHistoricalDefinition(ctx context.Context, sourceIdentifier string) (campaign.HistoricalDefinitionReceipt, bool, error) {
	if journal == nil || journal.tx == nil || !journal.scope.valid() {
		return campaign.HistoricalDefinitionReceipt{}, false, ErrInvalidScope
	}
	sourceKey, err := decodeSourceIdentifier(sourceIdentifier)
	if err != nil {
		return campaign.HistoricalDefinitionReceipt{}, false, err
	}
	tx, err := journal.tx(ctx)
	if err != nil {
		return campaign.HistoricalDefinitionReceipt{}, false, err
	}
	var payload []byte
	var targetID string
	var metadata []byte
	err = tx.QueryRow(ctx, `SELECT payload_digest,target_id,metadata FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4
  AND source_key_digest=$5 AND disposition='import'`, journal.scope.ImportVersion, journal.scope.ArchiveRunID,
		journal.scope.AdapterID, journal.scope.TableID, sourceKey).Scan(&payload, &targetID, &metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign.HistoricalDefinitionReceipt{}, false, nil
	}
	if err != nil {
		return campaign.HistoricalDefinitionReceipt{}, false, err
	}
	if len(payload) != sha256.Size {
		return campaign.HistoricalDefinitionReceipt{}, false, ErrConflict
	}
	var values struct {
		OriginalApprovalStatus string `json:"original_approval_status"`
		OriginalRuntimeStatus  string `json:"original_runtime_status"`
	}
	if json.Unmarshal(metadata, &values) != nil || values.OriginalApprovalStatus == "" || values.OriginalRuntimeStatus == "" {
		return campaign.HistoricalDefinitionReceipt{}, false, ErrConflict
	}
	var digest [sha256.Size]byte
	copy(digest[:], payload)
	return campaign.HistoricalDefinitionReceipt{
		SourceIdentifier: sourceIdentifier, PayloadDigest: digest,
		OriginalApprovalStatus: values.OriginalApprovalStatus,
		OriginalRuntimeStatus:  values.OriginalRuntimeStatus,
		TargetCampaignCode:     targetID,
	}, true, nil
}

func (journal *Journal) RecordHistoricalDefinition(ctx context.Context, receipt campaign.HistoricalDefinitionReceipt) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() || receipt.Replayed || receipt.TargetCampaignCode == "" {
		return ErrInvalidScope
	}
	sourceKey, err := decodeSourceIdentifier(receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]string{
		"original_approval_status": receipt.OriginalApprovalStatus,
		"original_runtime_status":  receipt.OriginalRuntimeStatus,
	})
	if err != nil {
		return err
	}
	targetDigest := sha256.Sum256([]byte(journal.scope.TargetDomain + "\x00" + journal.scope.TargetTable + "\x00" + receipt.TargetCampaignCode + "\x00" + hex.EncodeToString(receipt.PayloadDigest[:])))
	tx, err := journal.tx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.v1_domain_import_receipts
(import_version,archive_run_id,adapter_id,table_id,source_key_digest,payload_digest,disposition,
 target_domain,target_table,target_id,target_digest,metadata,verified)
VALUES ($1,$2,$3,$4,$5,$6,'import',$7,$8,$9,$10,$11,true)`, journal.scope.ImportVersion,
		journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID, sourceKey,
		receipt.PayloadDigest[:], journal.scope.TargetDomain, journal.scope.TargetTable,
		receipt.TargetCampaignCode, targetDigest[:], metadata)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return campaign.ErrHistoricalDefinitionConflict
		}
	}
	return err
}

// Record stores one already-verified terminal decision. Import receipts must
// identify a V2 target; archive/quarantine receipts deliberately cannot.
func (journal *Journal) Record(ctx context.Context, receipt TerminalReceipt) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() {
		return ErrInvalidScope
	}
	metadata, err := json.Marshal(receipt.Metadata)
	if err != nil {
		return err
	}
	tx, err := journal.tx(ctx)
	if err != nil {
		return err
	}
	var targetDomain, targetTable, targetID any
	var targetDigest any
	switch receipt.Disposition {
	case "import":
		if receipt.Reason != "" || receipt.TargetID == "" || receipt.TargetDigest == ([sha256.Size]byte{}) {
			return ErrInvalidScope
		}
		targetDomain, targetTable, targetID, targetDigest = journal.scope.TargetDomain, journal.scope.TargetTable, receipt.TargetID, receipt.TargetDigest[:]
	case "archive", "quarantine":
		if receipt.Reason == "" || receipt.TargetID != "" || receipt.TargetDigest != ([sha256.Size]byte{}) {
			return ErrInvalidScope
		}
	default:
		return ErrInvalidScope
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.v1_domain_import_receipts
(import_version,archive_run_id,adapter_id,table_id,source_key_digest,payload_digest,disposition,reason,
 target_domain,target_table,target_id,target_digest,metadata,verified)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,true)
ON CONFLICT (import_version,archive_run_id,adapter_id,table_id,source_key_digest) DO NOTHING`,
		journal.scope.ImportVersion, journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID,
		receipt.SourceKeyDigest[:], receipt.PayloadDigest[:], receipt.Disposition, receipt.Reason,
		targetDomain, targetTable, targetID, targetDigest, metadata)
	if err != nil {
		return err
	}
	var payload []byte
	var disposition, reason string
	var foundTargetID *string
	var foundTargetDigest []byte
	var foundMetadata []byte
	err = tx.QueryRow(ctx, `SELECT payload_digest,disposition,reason,target_id,target_digest,metadata FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4 AND source_key_digest=$5`,
		journal.scope.ImportVersion, journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID,
		receipt.SourceKeyDigest[:]).Scan(&payload, &disposition, &reason, &foundTargetID, &foundTargetDigest, &foundMetadata)
	if err != nil || len(payload) != sha256.Size || !equalDigest(payload, receipt.PayloadDigest) || disposition != receipt.Disposition || reason != receipt.Reason {
		return ErrConflict
	}
	if (receipt.Disposition == "import" && (foundTargetID == nil || *foundTargetID != receipt.TargetID || !equalDigest(foundTargetDigest, receipt.TargetDigest))) ||
		(receipt.Disposition != "import" && (foundTargetID != nil || foundTargetDigest != nil)) || string(foundMetadata) != string(metadata) {
		return ErrConflict
	}
	return nil
}

func equalDigest(value []byte, digest [sha256.Size]byte) bool {
	if len(value) != sha256.Size {
		return false
	}
	var found [sha256.Size]byte
	copy(found[:], value)
	return found == digest
}

func decodeSourceIdentifier(value string) ([]byte, error) {
	if value == "" || value != strings.TrimSpace(value) {
		return nil, ErrInvalidScope
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return nil, ErrInvalidScope
	}
	return decoded, nil
}

func validToken(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value)
}

func validTableID(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && validIdentifier(parts[0], 63) && validIdentifier(parts[1], 63)
}

func validIdentifier(value string, limit int) bool {
	if value == "" || len(value) > limit || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func SourceIdentifier(sourceKeyDigest [sha256.Size]byte) string {
	return hex.EncodeToString(sourceKeyDigest[:])
}

func ParseSourceIdentifier(value string) ([sha256.Size]byte, error) {
	decoded, err := decodeSourceIdentifier(value)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("parse V1 domain source identifier: %w", err)
	}
	var result [sha256.Size]byte
	copy(result[:], decoded)
	return result, nil
}
