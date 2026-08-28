package port

import "context"

// DM01CustomerRootLocker proves and locks a completed DM01 mapping, its matching
// receipt and active Customer in the caller's transaction. It never creates a root.
type DM01CustomerRootLocker interface {
	LockVerifiedDM01CustomerRoot(context.Context, int64, [32]byte) (CustomerID, bool, error)
}
