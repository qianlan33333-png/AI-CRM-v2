package port

import (
	"context"
	"errors"
	"time"
)

// Immutable V1 facts only. Source references are not current V2 identifiers.
var (
	ErrStaticProductHistoryInvalid     = errors.New("invalid product static history")
	ErrStaticProductHistoryConflict    = errors.New("product static history conflict")
	ErrStaticProductHistoryUnavailable = errors.New("product static history unavailable")
)

type HistoricalProductPageSlice struct {
	ID                  int64     `json:"id"`
	SourceID            int64     `json:"source_id"`
	SourceKeyDigest     [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
	ProductSourceID     int64     `json:"product_source_id"`
	ImageSourceID       int64     `json:"image_source_id"`
	SortOrder           int64     `json:"sort_order"`
	OriginalEnabled     bool      `json:"original_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type StaticProductHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type StaticProductHistoryQuery struct {
	Limit, Offset int32
}

// Writes and journal receipts must share the caller transaction.
type StaticProductHistoryStore interface {
	CreateHistoricalProductPageSlice(context.Context, HistoricalProductPageSlice) (HistoricalProductPageSlice, error)
	GetHistoricalProductPageSlice(context.Context, int64) (HistoricalProductPageSlice, error)
}
type StaticProductHistoryJournal interface {
	LoadStaticProductHistory(context.Context, string, string) (StaticProductHistoryReceipt, bool, error)
	RecordStaticProductHistory(context.Context, StaticProductHistoryReceipt) error
}
type StaticProductHistoryReader interface {
	GetHistoricalProductPageSlice(context.Context, int64) (HistoricalProductPageSlice, error)
	ListHistoricalProductPageSlice(context.Context, StaticProductHistoryQuery) ([]HistoricalProductPageSlice, int64, error)
}
