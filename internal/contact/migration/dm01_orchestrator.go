package migration

import (
	"bytes"
	"errors"
)

type RunMode string

const (
	ModePreflight   RunMode = "preflight"
	ModeFull        RunMode = "full"
	ModeIncremental RunMode = "incremental"
	ModeReconcile   RunMode = "reconcile"
)

var (
	ErrInvalidRunMode       = errors.New("invalid DM01 run mode")
	ErrSourcePayloadDrift   = errors.New("DM01 source payload drift")
	ErrInvalidSourceReceipt = errors.New("invalid DM01 source receipt")
)

func (m RunMode) Valid() bool {
	return m == ModePreflight || m == ModeFull || m == ModeIncremental || m == ModeReconcile
}

type RowReceipt struct {
	SourceTable string
	SourceKey   []byte
	PayloadHMAC []byte
	Disposition string
}

// RowReceiptStore is migration-owned and transaction-bound by its adapter.
// A receipt is appended before a target fact is made visible in that UoW.
type RowReceiptStore interface {
	FindRowReceipt(RowReceipt) (RowReceipt, bool, error)
	AppendRowReceipt(RowReceipt) error
}

// RecordRow returns true only when the caller owns first processing of the
// source row. Same key/payload is an idempotent replay; same key/different
// payload fails closed so operators can quarantine rather than overwrite.
func RecordRow(store RowReceiptStore, next RowReceipt) (bool, error) {
	if store == nil || next.SourceTable == "" || len(next.SourceKey) != 32 || len(next.PayloadHMAC) != 32 || next.Disposition == "" {
		return false, ErrInvalidSourceReceipt
	}
	prior, found, err := store.FindRowReceipt(next)
	if err != nil {
		return false, err
	}
	if found {
		if !bytes.Equal(prior.PayloadHMAC, next.PayloadHMAC) {
			return false, ErrSourcePayloadDrift
		}
		return false, nil
	}
	if err := store.AppendRowReceipt(next); err != nil {
		return false, err
	}
	return true, nil
}
