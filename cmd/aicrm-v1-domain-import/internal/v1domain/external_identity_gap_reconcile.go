package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// ReconcileExternalIdentityGap seals only the 63 authenticated archive rows
// absent from the completed DM01 receipt collection. It never writes the
// pre-existing DM01 receipt ledger.
func ReconcileExternalIdentityGap(ctx context.Context, pool *pgxpool.Pool, importer *ExternalIdentityGapImporter, options ExternalIdentityGapImportOptions) (ReconciliationResult, error) {
	if ctx == nil || pool == nil || importer == nil || !validExternalIdentityGapOptions(options) ||
		nilExternalIdentityGap(importer.archive) || nilExternalIdentityGap(importer.uow) || nilExternalIdentityGap(importer.target) ||
		nilExternalIdentityGap(importer.roots) || nilExternalIdentityGap(importer.receipts) || nilExternalIdentityGap(importer.journal) ||
		importer.journal.ValidateExternalIdentityGapScope(options) != nil {
		return ReconciliationResult{}, ErrInvalidScope
	}

	var result ReconciliationResult
	uow := platformstore.NewUnitOfWork(externalIdentityGapSerializableBeginner{pool: pool})
	err := uow.Within(ctx, func(txCtx context.Context) error {
		bound, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil || bound == nil {
			return ErrConflict
		}
		if _, lockErr := bound.Exec(txCtx, `LOCK TABLE public.v1_domain_import_receipts IN SHARE ROW EXCLUSIVE MODE`); lockErr != nil {
			return lockErr
		}
		selection, selectErr := SelectExternalIdentityGap(txCtx, importer.archive, importer.receipts, options)
		if selectErr != nil {
			return selectErr
		}
		if verifyErr := importer.Verify(txCtx, options); verifyErr != nil {
			return verifyErr
		}
		receipts, receiptErr := loadExternalIdentityGapReconciliationReceipts(txCtx, bound, importer.journal, selection, options)
		if receiptErr != nil {
			return receiptErr
		}
		digest, digestErr := validateExternalIdentityGapReconciliation(selection, receipts)
		if digestErr != nil {
			return digestErr
		}
		result = ReconciliationResult{
			SelectedSourceCount: int64(len(selection.OnlyArchive)), ReceiptCount: int64(len(receipts)),
			ImportedCount: int64(len(receipts)), VerifiedCount: int64(len(receipts)),
			ComparisonDigest: hex.EncodeToString(digest[:]),
		}
		command, insertErr := bound.Exec(txCtx, `INSERT INTO public.v1_domain_import_reconciliation_receipts
(import_version,archive_run_id,selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest)
VALUES ($1,$2,$3,$4,$5,0,0,$6,$7)
ON CONFLICT (import_version,archive_run_id) DO NOTHING`, externalIdentityGapImportVersion, options.ArchiveRunID,
			result.SelectedSourceCount, result.ReceiptCount, result.ImportedCount, result.VerifiedCount, digest[:])
		if insertErr != nil {
			return insertErr
		}
		result.Replayed = command.RowsAffected() == 0
		if !result.Replayed {
			return nil
		}
		var selected, receiptCount, imported, archived, quarantined, verified int64
		var existing []byte
		if replayErr := bound.QueryRow(txCtx, `SELECT selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest
FROM public.v1_domain_import_reconciliation_receipts WHERE import_version=$1 AND archive_run_id=$2`, externalIdentityGapImportVersion, options.ArchiveRunID).
			Scan(&selected, &receiptCount, &imported, &archived, &quarantined, &verified, &existing); replayErr != nil ||
			selected != result.SelectedSourceCount || receiptCount != result.ReceiptCount || imported != result.ImportedCount ||
			archived != 0 || quarantined != 0 || verified != result.VerifiedCount || !equalBytes(existing, digest[:]) {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return ReconciliationResult{}, err
	}
	return result, nil
}

type externalIdentityGapSerializableBeginner struct{ pool *pgxpool.Pool }

func (beginner externalIdentityGapSerializableBeginner) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	if beginner.pool == nil {
		return nil, ErrInvalidScope
	}
	return beginner.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
}

type externalIdentityGapReconciliationReceipt struct {
	SourceKeyDigest [sha256.Size]byte
	PayloadDigest   [sha256.Size]byte
	FieldDigest     [sha256.Size]byte
	Terminal        TerminalReceipt
}

func loadExternalIdentityGapReconciliationReceipts(ctx context.Context, tx pgx.Tx, journal ExternalIdentityGapImportJournal, selection ExternalIdentityGapSelection, options ExternalIdentityGapImportOptions) ([]externalIdentityGapReconciliationReceipt, error) {
	if ctx == nil || tx == nil || journal == nil || !validExternalIdentityGapOptions(options) {
		return nil, ErrInvalidScope
	}
	var count int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2`, externalIdentityGapImportVersion, options.ArchiveRunID).Scan(&count); err != nil {
		return nil, err
	}
	if count != int64(len(selection.OnlyArchive)) {
		return nil, ErrConflict
	}
	receipts := make([]externalIdentityGapReconciliationReceipt, 0, len(selection.OnlyArchive))
	for _, value := range selection.OnlyArchive {
		terminal, found, err := journal.LoadTerminal(ctx, SourceIdentifier(value.ArchivedRow.SourceKeyHMAC))
		if err != nil || !found {
			return nil, ErrConflict
		}
		receipts = append(receipts, externalIdentityGapReconciliationReceipt{
			SourceKeyDigest: value.ArchivedRow.SourceKeyHMAC, PayloadDigest: value.ArchivedRow.PayloadHMAC,
			FieldDigest: value.ArchivedRow.FieldHMAC, Terminal: terminal,
		})
	}
	return receipts, nil
}

func validateExternalIdentityGapReconciliation(selection ExternalIdentityGapSelection, receipts []externalIdentityGapReconciliationReceipt) ([sha256.Size]byte, error) {
	if selection.ArchiveRows <= 0 || selection.DM01TerminalRows < 0 || selection.DM01TerminalRows+len(selection.OnlyArchive) != selection.ArchiveRows ||
		len(selection.OnlyArchive) == 0 || selection.SummaryDigest == ([sha256.Size]byte{}) || len(receipts) != len(selection.OnlyArchive) {
		return [sha256.Size]byte{}, ErrConflict
	}
	expected := make(map[[sha256.Size]byte]ExternalIdentityGapRow, len(selection.OnlyArchive))
	for _, value := range selection.OnlyArchive {
		if value.ArchivedRow.AdapterID != v1archive.DefaultAdapterID || value.ArchivedRow.TableID != dm01ExternalIdentityArchiveTableID ||
			value.ArchivedRow.SourceKeyHMAC == ([sha256.Size]byte{}) || value.ArchivedRow.PayloadHMAC == ([sha256.Size]byte{}) || value.ArchivedRow.FieldHMAC == ([sha256.Size]byte{}) {
			return [sha256.Size]byte{}, ErrConflict
		}
		if _, duplicate := expected[value.ArchivedRow.SourceKeyHMAC]; duplicate {
			return [sha256.Size]byte{}, ErrConflict
		}
		expected[value.ArchivedRow.SourceKeyHMAC] = value
	}

	type sealedReceipt struct {
		SourceKey string          `json:"source_key"`
		Payload   string          `json:"payload"`
		Field     string          `json:"field"`
		TargetID  string          `json:"target_id"`
		Target    string          `json:"target_digest"`
		Metadata  json.RawMessage `json:"metadata"`
	}
	sealed := make([]sealedReceipt, 0, len(receipts))
	targets := make(map[string]struct{}, len(receipts))
	seen := make(map[[sha256.Size]byte]struct{}, len(receipts))
	for _, receipt := range receipts {
		value, found := expected[receipt.SourceKeyDigest]
		if !found || receipt.PayloadDigest != value.ArchivedRow.PayloadHMAC || receipt.FieldDigest != value.ArchivedRow.FieldHMAC ||
			receipt.Terminal.SourceKeyDigest != receipt.SourceKeyDigest || receipt.Terminal.PayloadDigest != receipt.PayloadDigest ||
			receipt.Terminal.Disposition != "import" || receipt.Terminal.Reason != "" || receipt.Terminal.TargetDigest == ([sha256.Size]byte{}) || receipt.Terminal.Metadata == nil {
			return [sha256.Size]byte{}, ErrConflict
		}
		if _, duplicate := seen[receipt.SourceKeyDigest]; duplicate {
			return [sha256.Size]byte{}, ErrConflict
		}
		seen[receipt.SourceKeyDigest] = struct{}{}
		id, err := positiveID(receipt.Terminal.TargetID)
		if err != nil || strconv.FormatInt(id, 10) != receipt.Terminal.TargetID {
			return [sha256.Size]byte{}, ErrConflict
		}
		if _, duplicate := targets[receipt.Terminal.TargetID]; duplicate {
			return [sha256.Size]byte{}, ErrConflict
		}
		targets[receipt.Terminal.TargetID] = struct{}{}
		metadata, err := json.Marshal(receipt.Terminal.Metadata)
		if err != nil || !json.Valid(metadata) {
			return [sha256.Size]byte{}, ErrConflict
		}
		sealed = append(sealed, sealedReceipt{SourceKey: hex.EncodeToString(receipt.SourceKeyDigest[:]), Payload: hex.EncodeToString(receipt.PayloadDigest[:]),
			Field: hex.EncodeToString(receipt.FieldDigest[:]), TargetID: receipt.Terminal.TargetID,
			Target: hex.EncodeToString(receipt.Terminal.TargetDigest[:]), Metadata: metadata})
	}
	if len(seen) != len(expected) {
		return [sha256.Size]byte{}, ErrConflict
	}
	sort.Slice(sealed, func(left, right int) bool { return sealed[left].SourceKey < sealed[right].SourceKey })
	payload, err := json.Marshal(struct {
		Summary  string          `json:"selection_summary_digest"`
		Receipts []sealedReceipt `json:"receipts"`
	}{Summary: hex.EncodeToString(selection.SummaryDigest[:]), Receipts: sealed})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("seal external identity gap: %w", err)
	}
	return sha256.Sum256(payload), nil
}
