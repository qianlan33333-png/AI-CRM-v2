package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var _ contactport.DM01CustomerRootLocker = HistoricalImportRepository{}

// LockVerifiedDM01CustomerRoot is a caller-transaction write prerequisite.
// It locks only an imported DM01 mapping whose receipt payload still matches
// an active Customer; it never creates or repairs a Customer root.
func (HistoricalImportRepository) LockVerifiedDM01CustomerRoot(ctx context.Context, runID int64, sourceKeyHMAC [32]byte) (contactport.CustomerID, bool, error) {
	if runID < 1 || sourceKeyHMAC == ([32]byte{}) {
		return 0, false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, false, err
	}
	customerID, err := contactdb.New(tx).LockVerifiedDM01CustomerRoot(ctx, contactdb.LockVerifiedDM01CustomerRootParams{RunID: runID, SourceKeyHmac: sourceKeyHMAC[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if customerID < 1 {
		return 0, false, ErrHistoricalImportTargetDrift
	}
	return contactport.CustomerID(customerID), true, nil
}
