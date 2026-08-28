package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	cyclestore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

var staticTailHistoryReconciledTables = []string{
	staticTailGroupInviteTable,
	staticTailPageSliceTable,
	staticTailStrategyTable,
	staticTailVersionTable,
	staticTailDocumentTable,
}

// ReconcileStaticTailHistory seals only the five immutable historical tables.
// It cannot attest to a current group invite, product page, or cycle runtime.
func ReconcileStaticTailHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != staticTailHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, staticTailHistoryReconciledTables)
}

func isStaticTailHistorySource(tableID string) bool {
	for _, table := range staticTailHistoryReconciledTables {
		if tableID == table {
			return true
		}
	}
	return false
}

// verifyStaticTailHistoryTarget is called by the central reconciler with the
// same transaction it uses for receipts, so the historical parent links are
// checked against the exact reconciled batch.
func verifyStaticTailHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	return verifyStaticTailHistoryRow(ctx,
		mediastore.NewStaticMediaHistoryReader(tx),
		productstore.NewStaticProductHistoryReader(tx),
		cyclestore.NewStaticCycleHistoryReader(tx),
		row, targets)
}

func verifyStaticTailHistoryRow(ctx context.Context, media mediaport.StaticMediaHistoryReader, product productport.StaticProductHistoryReader, cycle cycleport.StaticCycleHistoryReader, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	if !isStaticTailHistorySource(row.TableID) || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size ||
		row.TargetDomain == nil || row.TargetTable == nil || row.TargetID == nil || row.Disposition != "import" || row.Reason != "" || !row.Verified {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var digest [sha256.Size]byte
	switch row.TableID {
	case staticTailGroupInviteTable:
		if media == nil || *row.TargetDomain != "media" || *row.TargetTable != staticTailGroupInviteTarget {
			return "", ErrConflict
		}
		actual, readErr := media.GetHistoricalGroupInvite(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = mediaapp.HistoricalGroupInviteDigest(actual)
	case staticTailPageSliceTable:
		if product == nil || *row.TargetDomain != "product" || *row.TargetTable != staticTailPageSliceTarget {
			return "", ErrConflict
		}
		actual, readErr := product.GetHistoricalProductPageSlice(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = productapp.HistoricalProductPageSliceDigest(actual)
	case staticTailStrategyTable:
		if cycle == nil || *row.TargetDomain != "operationcycle" || *row.TargetTable != staticTailStrategyTarget {
			return "", ErrConflict
		}
		actual, readErr := cycle.GetHistoricalCycleStrategy(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = cycleapp.HistoricalCycleStrategyDigest(actual)
	case staticTailVersionTable:
		if cycle == nil || *row.TargetDomain != "operationcycle" || *row.TargetTable != staticTailVersionTarget {
			return "", ErrConflict
		}
		actual, readErr := cycle.GetHistoricalCycleVersion(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) ||
			!staticTailParentRecorded(targets, staticTailStrategyTarget, actual.StrategyHistoryID) {
			return "", ErrConflict
		}
		parent, parentErr := cycle.GetHistoricalCycleStrategy(ctx, actual.StrategyHistoryID)
		if parentErr != nil || parent.ID != actual.StrategyHistoryID || parent.SourceID != actual.StrategySourceID {
			return "", ErrConflict
		}
		digest, err = cycleapp.HistoricalCycleVersionDigest(actual)
	case staticTailDocumentTable:
		if cycle == nil || *row.TargetDomain != "operationcycle" || *row.TargetTable != staticTailDocumentTarget {
			return "", ErrConflict
		}
		actual, readErr := cycle.GetHistoricalCycleDocument(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) ||
			!staticTailParentRecorded(targets, staticTailVersionTarget, actual.VersionHistoryID) {
			return "", ErrConflict
		}
		parent, parentErr := cycle.GetHistoricalCycleVersion(ctx, actual.VersionHistoryID)
		if parentErr != nil || parent.ID != actual.VersionHistoryID || parent.SourceID != actual.StrategyVersionSourceID {
			return "", ErrConflict
		}
		digest, err = cycleapp.HistoricalCycleDocumentDigest(actual)
	default:
		return "", ErrConflict
	}
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

func staticTailParentRecorded(targets map[string]map[string]struct{}, table string, id int64) bool {
	if id < 1 || targets == nil {
		return false
	}
	_, found := targets[table][strconv.FormatInt(id, 10)]
	return found
}
