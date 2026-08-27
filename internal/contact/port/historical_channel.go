package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrHistoricalChannelInvalid     = errors.New("invalid historical channel")
	ErrHistoricalChannelConflict    = errors.New("historical channel conflict")
	ErrHistoricalChannelUnavailable = errors.New("historical channel unavailable")
)

// HistoricalChannelDefinition contains only a local, inactive definition. No
// V1 asset, callback, welcome message or staff identifier is executable here.
type HistoricalChannelDefinition struct {
	SourceIdentifier                                         string
	PayloadDigest                                            [32]byte
	Code, Name, ChannelType, CarrierType, LegacyConfigDigest string
	Actor                                                    int64
	CreatedAt, UpdatedAt                                     time.Time
}

type HistoricalChannelRecord struct {
	ID                   int64
	Code, Name, Status   string
	Projection           json.RawMessage
	LegacyConfigDigest   string
	CreatedBy, UpdatedBy int64
	CreatedAt, UpdatedAt time.Time
}

type HistoricalChannelReceipt struct {
	SourceIdentifier            string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

// Both interfaces require the same caller-bound transaction. The importer
// owns provenance; Contact alone owns channels and legacy asset digests.
type HistoricalChannelStore interface {
	CreateHistoricalChannel(context.Context, HistoricalChannelRecord) (HistoricalChannelRecord, error)
	GetHistoricalChannel(context.Context, int64) (HistoricalChannelRecord, error)
}

type HistoricalChannelJournal interface {
	LoadHistoricalChannel(context.Context, string) (HistoricalChannelReceipt, bool, error)
	RecordHistoricalChannel(context.Context, HistoricalChannelReceipt) error
}
