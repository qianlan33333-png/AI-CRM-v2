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
type HistoricalInvalidRadarLink struct {
	ID                   int64     `json:"id"`
	SourceKeyDigest      [32]byte  `json:"-"`
	SourcePayloadDigest  [32]byte  `json:"-"`
	SourceFieldDigest    [32]byte  `json:"-"`
	PrivateDigest        [32]byte  `json:"-"`
	RedactedRoots        []string  `json:"-"`
	SourceID             int64     `json:"source_id"`
	Code                 string    `json:"code"`
	Title                string    `json:"title"`
	DestinationURLDigest [32]byte  `json:"-"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	QuarantineReason     string    `json:"quarantine_reason"`
}

type InvalidSourceHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type InvalidSourceHistoryQuery struct{ Limit, Offset int32 }

type InvalidSourceHistoryStore interface {
	CreateHistoricalInvalidRadarLink(context.Context, HistoricalInvalidRadarLink) (HistoricalInvalidRadarLink, error)
	GetHistoricalInvalidRadarLink(context.Context, int64) (HistoricalInvalidRadarLink, error)
}
type InvalidSourceHistoryReader interface {
	GetHistoricalInvalidRadarLink(context.Context, int64) (HistoricalInvalidRadarLink, error)
	ListHistoricalInvalidRadarLink(context.Context, InvalidSourceHistoryQuery) ([]HistoricalInvalidRadarLink, int64, error)
}
type InvalidSourceHistoryJournal interface {
	LoadInvalidSourceHistory(context.Context, string, string) (InvalidSourceHistoryReceipt, bool, error)
	RecordInvalidSourceHistory(context.Context, InvalidSourceHistoryReceipt) error
}
