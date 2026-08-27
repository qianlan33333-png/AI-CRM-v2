package v1domain

import (
	"context"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// Finance must already be sealed before a missing order mapping can be treated
// as unresolved historical data rather than an import-order mistake.
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
