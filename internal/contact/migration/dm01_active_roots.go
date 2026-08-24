package migration

import "errors"

var ErrMissingCustomerRoot = errors.New("DM01 external identity missing customer root")

// ActiveRootProcessor contains only the ordering invariant shared by the
// 230/152/314 adapters: a 314 row can bind only to an already mapped 152 root.
type ActiveRootProcessor struct {
	Receipts RowReceiptStore
}

func (p ActiveRootProcessor) BeginRow(receipt RowReceipt) (bool, error) {
	return RecordRow(p.Receipts, receipt)
}

func (p ActiveRootProcessor) RequireCustomerRoot(mappedCustomerID int64) error {
	if mappedCustomerID < 1 {
		return ErrMissingCustomerRoot
	}
	return nil
}
