package migration

import (
	"context"
	"time"
)

type LeaseFence struct {
	RunID      RunID
	Generation uint64
	Token      Digest
	ExpiresAt  time.Time
}

func (fence LeaseFence) valid() bool {
	return fence.RunID != "" && fence.Generation > 0 && fence.Token != (Digest{}) && !fence.ExpiresAt.IsZero()
}

type TableCheckpoint struct {
	UpperBound UpperBound
	Cursor     Cursor
	Processed  uint64
	Complete   bool
}

type RunPhase string

const (
	PhaseRunning    RunPhase = "running"
	PhaseCompleted  RunPhase = "completed"
	PhaseReconciled RunPhase = "reconciled"
)

type RunState struct {
	ID             RunID
	Adapter        AdapterID
	ManifestDigest Digest
	Phase          RunPhase
	Tables         map[TableID]TableCheckpoint
}

type StartRun struct {
	ID             RunID
	Adapter        AdapterID
	ManifestDigest Digest
	Bounds         []TableBound
}

type TargetUnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

type RunStore interface {
	Open(context.Context, StartRun) (RunState, error)
	Load(context.Context, RunID) (RunState, error)
	// AcquireLease must atomically reject a non-expired or malformed existing
	// lease. A successful takeover creates a new generation/token and expiry.
	AcquireLease(context.Context, RunID, time.Time, time.Duration) (LeaseFence, error)
	Advance(context.Context, LeaseFence, TableID, TableCheckpoint) error
	Finish(context.Context, LeaseFence) error
	MarkReconciled(context.Context, LeaseFence) error
}

type RowReceipt struct {
	Adapter     AdapterID
	Table       TableID
	SourceKey   Digest
	Payload     Digest
	Field       Digest
	Disposition Disposition
	Mutation    Digest
}

type RowReceiptStore interface {
	FindRowReceipt(context.Context, AdapterID, TableID, Digest) (RowReceipt, bool, error)
	AppendRowReceipt(context.Context, LeaseFence, RowReceipt) error
}

type TargetWriter interface {
	Apply(context.Context, LeaseFence, MappedRow) error
}

type Quarantine struct {
	Adapter   AdapterID
	Table     TableID
	SourceKey Digest
	Payload   Digest
	Field     Digest
	Reason    string
}

type QuarantineWriter interface {
	Quarantine(context.Context, LeaseFence, Quarantine) error
}

type Archive struct {
	Adapter   AdapterID
	Table     TableID
	SourceKey Digest
	Payload   Digest
	Field     Digest
}

type ArchiveWriter interface {
	Archive(context.Context, LeaseFence, Archive) error
}

type ResultReceipt struct {
	RunID RunID
	RowReceipt
	Outcome        Disposition
	MutationDigest Digest
}

type ResultReceiptStore interface {
	FindResultReceipt(context.Context, RunID, AdapterID, TableID, Digest) (ResultReceipt, bool, error)
	AppendResultReceipt(context.Context, LeaseFence, ResultReceipt) error
	ListResultReceipts(context.Context, RunID) ([]ResultReceipt, error)
}

type TargetVerifier interface {
	VerifyResultReceipt(context.Context, ResultReceipt) error
}
