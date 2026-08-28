// Package v1domain records V2-owned, immutable provenance for safe canonical
// imports from the encrypted V1 archive. It never stores source payloads.
package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	deferredhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var (
	ErrInvalidScope = errors.New("invalid V1 domain import scope")
	ErrConflict     = errors.New("V1 domain import receipt conflict")
)

type CampaignDefinitionReceiptReader struct{ pool *pgxpool.Pool }

func NewCampaignDefinitionReceiptReader(pool *pgxpool.Pool) *CampaignDefinitionReceiptReader {
	return &CampaignDefinitionReceiptReader{pool: pool}
}

func (reader *CampaignDefinitionReceiptReader) EachCampaignDefinitionPriorReceipt(ctx context.Context, run, table string, emit func(CampaignDefinitionPriorReceipt) error) error {
	if reader == nil || reader.pool == nil || ctx == nil || run == "" || emit == nil || (table != campaignTableID && table != campaignStepTableID) {
		return ErrInvalidScope
	}
	tx, err := reader.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT import_version,archive_run_id,adapter_id,table_id,COALESCE(target_domain,''),COALESCE(target_table,''),
source_key_digest,payload_digest,disposition,reason
FROM public.v1_domain_import_receipts
WHERE import_version='v1-domain-a1' AND archive_run_id=$1 AND table_id=$2
AND verified
AND EXISTS(SELECT 1 FROM public.v1_domain_import_reconciliation_receipts s WHERE s.import_version='v1-domain-a1'
AND s.archive_run_id=$1 AND s.selected_source_count=s.receipt_count AND s.verified_count=s.receipt_count)
AND (disposition='import' OR (target_domain IS NULL AND target_table IS NULL AND target_id IS NULL AND target_digest IS NULL))
ORDER BY source_key_digest`, run, table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value CampaignDefinitionPriorReceipt
		var key, payload []byte
		if err := rows.Scan(&value.ImportVersion, &value.ArchiveRunID, &value.AdapterID, &value.TableID, &value.TargetDomain, &value.TargetTable, &key, &payload, &value.Disposition, &value.Reason); err != nil {
			return err
		}
		if len(key) != sha256.Size || len(payload) != sha256.Size {
			return ErrConflict
		}
		copy(value.SourceKey[:], key)
		copy(value.PayloadDigest[:], payload)
		if err := emit(value); err != nil {
			return err
		}
	}
	return rows.Err()
}

type CampaignDefinitionCurrentResolver struct {
	run        string
	repository campaignport.CampaignDefinitionCurrentReader
	boundTx    pgx.Tx
}

func NewCampaignDefinitionCurrentResolver(run string, repository campaignport.CampaignDefinitionCurrentReader) *CampaignDefinitionCurrentResolver {
	return &CampaignDefinitionCurrentResolver{run: run, repository: repository}
}

// Reconciliation already owns a serializable transaction. The owner reader
// supplied to this copy must use that same transaction.
func (resolver *CampaignDefinitionCurrentResolver) WithTx(tx pgx.Tx) *CampaignDefinitionCurrentResolver {
	copy := *resolver
	copy.boundTx = tx
	return &copy
}

// A code is used only after exact source provenance and an actual Campaign
// read agree. No V1 numeric ID is treated as a V2 key.
func (resolver *CampaignDefinitionCurrentResolver) ResolveVerifiedCurrentCampaignDefinition(ctx context.Context, _ int64, sourceKey [sha256.Size]byte) (string, bool, error) {
	if resolver == nil || resolver.run == "" || resolver.repository == nil || sourceKey == ([sha256.Size]byte{}) {
		return "", false, ErrInvalidScope
	}
	tx := resolver.boundTx
	if tx == nil {
		var err error
		tx, err = platformstore.TxFromContext(ctx)
		if err != nil {
			return "", false, err
		}
	}
	var code *string
	var payload, targetDigest []byte
	var disposition string
	var verified, archived, sealed bool
	err := tx.QueryRow(ctx, `SELECT r.target_id,r.payload_digest,r.target_digest,r.disposition,r.verified,
EXISTS(SELECT 1 FROM public.v1_archive_records a WHERE a.run_id=r.archive_run_id AND a.adapter_id=r.adapter_id
AND a.table_id=r.table_id AND a.source_key_digest=r.source_key_digest AND a.payload_digest=r.payload_digest),
EXISTS(SELECT 1 FROM public.v1_domain_import_reconciliation_receipts s WHERE s.archive_run_id=r.archive_run_id
AND s.import_version=r.import_version AND s.selected_source_count=s.receipt_count AND s.verified_count=s.receipt_count)
FROM public.v1_domain_import_receipts r WHERE r.import_version='v1-domain-a1' AND r.archive_run_id=$1
AND r.adapter_id=$2 AND r.table_id='public/campaigns' AND r.source_key_digest=$3
AND r.target_domain='campaign' AND r.target_table='cloud_campaigns'`, resolver.run, v1archive.DefaultAdapterID, sourceKey[:]).Scan(&code, &payload, &targetDigest, &disposition, &verified, &archived, &sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !archived || !sealed || !verified {
		return "", false, ErrConflict
	}
	if disposition == "archive" || disposition == "quarantine" {
		return "", false, nil
	}
	if disposition != "import" || code == nil || *code == "" || len(payload) != sha256.Size || len(targetDigest) != sha256.Size {
		return "", false, ErrConflict
	}
	expected := sha256.Sum256([]byte("campaign\x00cloud_campaigns\x00" + *code + "\x00" + hex.EncodeToString(payload)))
	if !equalBytes(expected[:], targetDigest) {
		return "", false, ErrConflict
	}
	actual, err := resolver.repository.GetCurrentCampaignDefinitionHistoryParent(ctx, *code)
	if err != nil {
		return "", false, err
	}
	if actual.Code != *code || actual.ApprovalStatus != "rejected" || actual.RuntimeStatus != "paused" || actual.Version != 1 {
		return "", false, ErrConflict
	}
	return *code, true, nil
}

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
var _ media.HistoricalMiniProgramJournal = (*Journal)(nil)

// DeferredIdentityDM01Reader reads only immutable evidence already on V2.
// The caller supplies a transaction-reusing UoW for reconciliation.
type DeferredIdentityDM01Reader struct{ UOW UnitOfWork }

var _ deferredhistory.DM01EvidenceReader = DeferredIdentityDM01Reader{}

func (reader DeferredIdentityDM01Reader) read(ctx context.Context, run int64, table string, apply func(pgx.Tx) error) error {
	if reader.UOW == nil || ctx == nil || run < 1 || (table != "" && table != deferredhistory.DM01PeopleSourceTable && table != deferredhistory.DM01IdentityConflictsSourceTable && table != deferredhistory.DM01IdentityMapSourceTable) {
		return ErrInvalidScope
	}
	return reader.UOW.Within(ctx, func(bound context.Context) error {
		tx, err := platformstore.TxFromContext(bound)
		if err != nil {
			return err
		}
		return apply(tx)
	})
}

func (reader DeferredIdentityDM01Reader) ReadDM01Run(ctx context.Context, run int64) (value deferredhistory.DM01Run, err error) {
	err = reader.read(ctx, run, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id,mode,state,hmac_key_version FROM public.legacy_contact_identity_import_runs WHERE id=$1`, run).Scan(&value.ID, &value.Mode, &value.State, &value.HMACKeyVersion)
	})
	return
}

func (reader DeferredIdentityDM01Reader) ReadDM01Checkpoint(ctx context.Context, run int64, table string) (value deferredhistory.DM01Checkpoint, err error) {
	err = reader.read(ctx, run, table, func(tx pgx.Tx) error {
		var key, payload, field, upper []byte
		if err := tx.QueryRow(ctx, `SELECT source_table,final_source_key_hmac,payload_hmac,field_digest,watermark,upper_source_key_hmac,upper_bound_empty FROM public.legacy_contact_identity_import_checkpoints WHERE run_id=$1 AND source_table=$2`, run, table).Scan(&value.SourceTable, &key, &payload, &field, &value.Watermark, &upper, &value.UpperBoundEmpty); err != nil {
			return err
		}
		if len(key) != 32 || len(payload) != 32 || len(field) != 32 || len(upper) != 32 {
			return ErrConflict
		}
		copy(value.FinalSourceKeyHMAC[:], key)
		copy(value.PayloadHMAC[:], payload)
		copy(value.FieldDigest[:], field)
		copy(value.UpperSourceKeyHMAC[:], upper)
		return nil
	})
	return
}

func (reader DeferredIdentityDM01Reader) EachDM01TerminalReceipt(ctx context.Context, run int64, table string, emit func(deferredhistory.DM01TerminalReceipt) error) error {
	if emit == nil {
		return ErrInvalidScope
	}
	return reader.read(ctx, run, table, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT source_table,source_ordinal,source_key_hmac,payload_hmac,field_digest,disposition FROM public.legacy_contact_identity_import_row_receipts WHERE run_id=$1 AND source_table=$2 ORDER BY source_ordinal`, run, table)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value deferredhistory.DM01TerminalReceipt
			var key, payload, field []byte
			if err := rows.Scan(&value.SourceTable, &value.SourceOrdinal, &key, &payload, &field, &value.Disposition); err != nil {
				return err
			}
			if len(key) != 32 || len(payload) != 32 || len(field) != 32 {
				return ErrConflict
			}
			copy(value.SourceKeyHMAC[:], key)
			copy(value.PayloadHMAC[:], payload)
			copy(value.FieldDigest[:], field)
			if err := emit(value); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

func (reader DeferredIdentityDM01Reader) EachDM01Quarantine(ctx context.Context, run int64, table string, emit func(deferredhistory.DM01Quarantine) error) error {
	if emit == nil {
		return ErrInvalidScope
	}
	return reader.read(ctx, run, table, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT source_table,source_key_hmac,payload_hmac,field_digest,reason_code FROM public.legacy_contact_identity_import_quarantines WHERE run_id=$1 AND source_table=$2 ORDER BY source_key_hmac,reason_code`, run, table)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value deferredhistory.DM01Quarantine
			var key, payload, field []byte
			if err := rows.Scan(&value.SourceTable, &key, &payload, &field, &value.ReasonCode); err != nil {
				return err
			}
			if len(key) != 32 || len(payload) != 32 || len(field) != 32 {
				return ErrConflict
			}
			copy(value.SourceKeyHMAC[:], key)
			copy(value.PayloadHMAC[:], payload)
			copy(value.FieldDigest[:], field)
			if err := emit(value); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

// Finance must be sealed before missing order mappings can be treated as
// unresolved historical data rather than an import-order mistake.
func VerifyServicePeriodFinancePrerequisite(ctx context.Context, run string) error {
	if run == "" {
		return ErrInvalidScope
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var ready bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.v1_domain_import_reconciliation_receipts
WHERE archive_run_id=$1 AND import_version='v1-finance-a1' AND selected_source_count=receipt_count AND verified_count=receipt_count)`, run).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return ErrConflict
	}
	return nil
}

// VerifyFinanceReferencePrerequisites prevents sealing unresolved references
// merely because the earlier canonical package has not been imported yet.
func VerifyFinanceReferencePrerequisites(ctx context.Context, archiveRun string, dm01Run int64) error {
	if archiveRun == "" || dm01Run < 1 {
		return ErrInvalidScope
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var ready bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.legacy_contact_identity_import_runs
WHERE id=$1 AND mode='full' AND state='imported') AND EXISTS(
SELECT 1 FROM public.v1_domain_import_reconciliation_receipts WHERE import_version='v1-static-a1'
AND archive_run_id=$2 AND selected_source_count=receipt_count AND verified_count=receipt_count)`, dm01Run, archiveRun).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return ErrConflict
	}
	return nil
}

// LoadFinanceProductReference reads only sealed migration provenance, not a
// product-code guess or a legacy numeric ID reused as a V2 foreign key.
func LoadFinanceProductReference(ctx context.Context, archiveRun, code string) (id string, payload, digest, metadata []byte, found bool, err error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return "", nil, nil, nil, false, err
	}
	rows, err := tx.Query(ctx, `SELECT receipt.target_id,receipt.payload_digest,receipt.target_digest,receipt.metadata,
EXISTS(SELECT 1 FROM public.v1_archive_records archive WHERE archive.run_id=receipt.archive_run_id
AND archive.adapter_id=receipt.adapter_id AND archive.table_id=receipt.table_id
AND archive.source_key_digest=receipt.source_key_digest AND archive.payload_digest=receipt.payload_digest)
FROM public.v1_domain_import_receipts receipt
WHERE receipt.import_version='v1-static-a1' AND receipt.archive_run_id=$1 AND receipt.adapter_id=$2
AND receipt.table_id='public/wechat_pay_products' AND receipt.disposition='import' AND receipt.verified
AND receipt.target_domain='product' AND receipt.target_table='products'
AND receipt.metadata->>'target_product_code'=$3 LIMIT 2`, archiveRun, v1archive.DefaultAdapterID, code)
	if err != nil {
		return "", nil, nil, nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		if found {
			return "", nil, nil, nil, false, ErrConflict
		}
		var verified bool
		if err = rows.Scan(&id, &payload, &digest, &metadata, &verified); err != nil {
			return "", nil, nil, nil, false, err
		}
		if !verified {
			return "", nil, nil, nil, false, ErrConflict
		}
		found = true
	}
	err = rows.Err()
	return
}

// VerifySurveyUnresolvedSource preserves the old quarantine, and requires the
// exact archived payload before a separate source-history fact may be written.
func VerifySurveyUnresolvedSource(ctx context.Context, run string, row v1archive.ArchivedRow) error {
	if surveyUnresolvedTargets[row.TableID] == "" {
		return ErrInvalidScope
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var matches bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(
SELECT 1 FROM public.v1_domain_import_receipts old
JOIN public.v1_archive_records archive ON archive.run_id=old.archive_run_id AND archive.adapter_id=old.adapter_id
AND archive.table_id=old.table_id AND archive.source_key_digest=old.source_key_digest AND archive.payload_digest=old.payload_digest
WHERE old.import_version='v1-domain-a1' AND old.archive_run_id=$1 AND old.adapter_id=$2 AND old.table_id=$3
AND old.source_key_digest=$4 AND old.payload_digest=$5 AND archive.field_digest=$6
AND old.verified AND old.disposition='quarantine' AND old.reason='survey_definition_history_unresolved')`,
		run, v1archive.DefaultAdapterID, row.TableID, row.SourceKeyHMAC[:], row.PayloadHMAC[:], row.FieldHMAC[:]).Scan(&matches)
	if err != nil {
		return err
	}
	if !matches {
		return ErrConflict
	}
	return nil
}

// VerifyOutboundTaskHistoryPrerequisite prevents import order from turning a
// not-yet-imported broadcast parent into a permanently unresolved relation.
func VerifyOutboundTaskHistoryPrerequisite(ctx context.Context, run string) error {
	if run == "" {
		return ErrInvalidScope
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var ready bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.v1_domain_import_reconciliation_receipts
WHERE archive_run_id=$1 AND import_version='v1-broadcast-job-history-a1'
AND selected_source_count=receipt_count AND imported_count=receipt_count
AND verified_count=receipt_count AND archived_count=0 AND quarantined_count=0)`, run).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return ErrConflict
	}
	return nil
}
func NewJournal(scope Scope) (*Journal, error) {
	if !scope.valid() {
		return nil, ErrInvalidScope
	}
	return &Journal{scope: scope, tx: platformstore.TxFromContext}, nil
}

// LoadTerminal serializes one source identity, including the not-yet-imported
// case. Callers must use the same transaction for the target write and Record.
func (journal *Journal) LoadTerminal(ctx context.Context, sourceIdentifier string) (TerminalReceipt, bool, error) {
	if journal == nil || journal.tx == nil || !journal.scope.valid() {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	sourceKey, err := decodeSourceIdentifier(sourceIdentifier)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	tx, err := journal.tx(ctx)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	lockKey := strings.Join([]string{journal.scope.ImportVersion, journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID, sourceIdentifier}, "/")
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return TerminalReceipt{}, false, err
	}
	var receipt TerminalReceipt
	var payload, targetDigest, metadata []byte
	var targetDomain, targetTable, targetID *string
	var verified bool
	err = tx.QueryRow(ctx, `SELECT payload_digest,disposition,reason,target_domain,target_table,target_id,target_digest,metadata,verified
FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4 AND source_key_digest=$5`,
		journal.scope.ImportVersion, journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID, sourceKey).
		Scan(&payload, &receipt.Disposition, &receipt.Reason, &targetDomain, &targetTable, &targetID, &targetDigest, &metadata, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return TerminalReceipt{}, false, nil
	}
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	if len(payload) != sha256.Size || !verified {
		return TerminalReceipt{}, false, ErrConflict
	}
	copy(receipt.SourceKeyDigest[:], sourceKey)
	copy(receipt.PayloadDigest[:], payload)
	switch receipt.Disposition {
	case "import":
		if receipt.Reason != "" || targetDomain == nil || *targetDomain != journal.scope.TargetDomain || targetTable == nil || *targetTable != journal.scope.TargetTable || targetID == nil || *targetID == "" || len(targetDigest) != sha256.Size {
			return TerminalReceipt{}, false, ErrConflict
		}
		receipt.TargetID = *targetID
		copy(receipt.TargetDigest[:], targetDigest)
	case "archive", "quarantine":
		if receipt.Reason == "" || targetDomain != nil || targetTable != nil || targetID != nil || targetDigest != nil {
			return TerminalReceipt{}, false, ErrConflict
		}
	default:
		return TerminalReceipt{}, false, ErrConflict
	}
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.UseNumber()
	if decoder.Decode(&receipt.Metadata) != nil || receipt.Metadata == nil {
		return TerminalReceipt{}, false, ErrConflict
	}
	return receipt, true, nil
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

func (journal *Journal) LoadHistoricalMiniProgram(ctx context.Context, sourceIdentifier string) (media.HistoricalMiniProgramReceipt, bool, error) {
	if journal == nil || journal.tx == nil || !journal.scope.valid() {
		return media.HistoricalMiniProgramReceipt{}, false, ErrInvalidScope
	}
	sourceKey, err := decodeSourceIdentifier(sourceIdentifier)
	if err != nil {
		return media.HistoricalMiniProgramReceipt{}, false, err
	}
	tx, err := journal.tx(ctx)
	if err != nil {
		return media.HistoricalMiniProgramReceipt{}, false, err
	}
	var payload []byte
	var targetID string
	var metadata []byte
	err = tx.QueryRow(ctx, `SELECT payload_digest,target_id,metadata FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4
  AND source_key_digest=$5 AND disposition='import'`, journal.scope.ImportVersion, journal.scope.ArchiveRunID,
		journal.scope.AdapterID, journal.scope.TableID, sourceKey).Scan(&payload, &targetID, &metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return media.HistoricalMiniProgramReceipt{}, false, nil
	}
	if err != nil || len(payload) != sha256.Size {
		return media.HistoricalMiniProgramReceipt{}, false, ErrConflict
	}
	var values struct {
		SourceID                int64 `json:"source_id"`
		ProviderMaterialDropped bool  `json:"provider_material_dropped"`
	}
	if json.Unmarshal(metadata, &values) != nil || values.SourceID < 1 {
		return media.HistoricalMiniProgramReceipt{}, false, ErrConflict
	}
	parsedTargetID, err := strconv.ParseInt(targetID, 10, 64)
	if err != nil || parsedTargetID < 1 {
		return media.HistoricalMiniProgramReceipt{}, false, ErrConflict
	}
	var digest [sha256.Size]byte
	copy(digest[:], payload)
	return media.HistoricalMiniProgramReceipt{
		SourceIdentifier: sourceIdentifier, SourceID: values.SourceID, PayloadDigest: digest,
		TargetMiniProgramID: parsedTargetID, ProviderMaterialDropped: values.ProviderMaterialDropped,
	}, true, nil
}

func (journal *Journal) RecordHistoricalMiniProgram(ctx context.Context, receipt media.HistoricalMiniProgramReceipt) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() || receipt.Replayed || receipt.SourceID < 1 || receipt.TargetMiniProgramID < 1 {
		return ErrInvalidScope
	}
	sourceKey, err := decodeSourceIdentifier(receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"source_id": receipt.SourceID, "provider_material_dropped": receipt.ProviderMaterialDropped,
	})
	if err != nil {
		return err
	}
	targetID := strconv.FormatInt(receipt.TargetMiniProgramID, 10)
	targetDigest := sha256.Sum256([]byte(journal.scope.TargetDomain + "\x00" + journal.scope.TargetTable + "\x00" + targetID + "\x00" + hex.EncodeToString(receipt.PayloadDigest[:])))
	tx, err := journal.tx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.v1_domain_import_receipts
(import_version,archive_run_id,adapter_id,table_id,source_key_digest,payload_digest,disposition,
 target_domain,target_table,target_id,target_digest,metadata,verified)
VALUES ($1,$2,$3,$4,$5,$6,'import',$7,$8,$9,$10,$11,true)`, journal.scope.ImportVersion,
		journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID, sourceKey,
		receipt.PayloadDigest[:], journal.scope.TargetDomain, journal.scope.TargetTable, targetID, targetDigest[:], metadata)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return media.ErrHistoricalMiniProgramConflict
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
	metadata, err := marshalReceiptMetadata(receipt.Metadata)
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
	_, found, err := journal.LoadTerminal(ctx, SourceIdentifier(receipt.SourceKeyDigest))
	if err != nil {
		return err
	}
	if !found {
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
	}
	var payload []byte
	var disposition, reason string
	var foundTargetID *string
	var foundTargetDigest []byte
	var metadataMatches bool
	err = tx.QueryRow(ctx, `SELECT payload_digest,disposition,reason,target_id,target_digest,metadata=$6::jsonb FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4 AND source_key_digest=$5`,
		journal.scope.ImportVersion, journal.scope.ArchiveRunID, journal.scope.AdapterID, journal.scope.TableID,
		receipt.SourceKeyDigest[:], metadata).Scan(&payload, &disposition, &reason, &foundTargetID, &foundTargetDigest, &metadataMatches)
	if err != nil || len(payload) != sha256.Size || !equalDigest(payload, receipt.PayloadDigest) || disposition != receipt.Disposition || reason != receipt.Reason {
		return ErrConflict
	}
	if (receipt.Disposition == "import" && (foundTargetID == nil || *foundTargetID != receipt.TargetID || !equalDigest(foundTargetDigest, receipt.TargetDigest))) ||
		(receipt.Disposition != "import" && (foundTargetID != nil || foundTargetDigest != nil)) || !metadataMatches {
		return ErrConflict
	}
	return nil
}

func marshalReceiptMetadata(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
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
