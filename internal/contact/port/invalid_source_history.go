package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidSourceHistoryInvalid     = errors.New("invalid source history")
	ErrInvalidSourceHistoryConflict    = errors.New("source history conflict")
	ErrInvalidSourceHistoryUnavailable = errors.New("source history unavailable")
)

// Source-only facts. Never bind a Customer, activate a definition, serve content,
// or invoke a Provider. Private digests remain excluded from API JSON.
type HistoricalUnboundTag struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	PrivateDigest       [32]byte  `json:"-"`
	RedactedRoots       []string  `json:"-"`
	TagSourceID         string    `json:"tag_source_id"`
	UnionIDDigest       [32]byte  `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	QuarantineReason    string    `json:"quarantine_reason"`
}

type HistoricalInvalidChannel struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	PrivateDigest       [32]byte  `json:"-"`
	RedactedRoots       []string  `json:"-"`
	SourceID            int64     `json:"source_id"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	ChannelType         string    `json:"channel_type"`
	CarrierType         string    `json:"carrier_type"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	QuarantineReason    string    `json:"quarantine_reason"`
}

type InvalidSourceHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type InvalidSourceHistoryQuery struct{ Limit, Offset int32 }

type InvalidSourceHistoryStore interface {
	CreateHistoricalUnboundTag(context.Context, HistoricalUnboundTag) (HistoricalUnboundTag, error)
	GetHistoricalUnboundTag(context.Context, int64) (HistoricalUnboundTag, error)
	CreateHistoricalInvalidChannel(context.Context, HistoricalInvalidChannel) (HistoricalInvalidChannel, error)
	GetHistoricalInvalidChannel(context.Context, int64) (HistoricalInvalidChannel, error)
}
type InvalidSourceHistoryReader interface {
	GetHistoricalUnboundTag(context.Context, int64) (HistoricalUnboundTag, error)
	ListHistoricalUnboundTag(context.Context, InvalidSourceHistoryQuery) ([]HistoricalUnboundTag, int64, error)
	GetHistoricalInvalidChannel(context.Context, int64) (HistoricalInvalidChannel, error)
	ListHistoricalInvalidChannel(context.Context, InvalidSourceHistoryQuery) ([]HistoricalInvalidChannel, int64, error)
}
type InvalidSourceHistoryJournal interface {
	LoadInvalidSourceHistory(context.Context, string, string) (InvalidSourceHistoryReceipt, bool, error)
	RecordInvalidSourceHistory(context.Context, InvalidSourceHistoryReceipt) error
}
