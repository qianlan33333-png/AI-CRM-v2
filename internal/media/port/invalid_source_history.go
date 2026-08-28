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
type HistoricalInvalidAsset struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	PrivateDigest       [32]byte  `json:"-"`
	RedactedRoots       []string  `json:"-"`
	Kind                string    `json:"kind"`
	SourceID            int64     `json:"source_id"`
	Name                string    `json:"name"`
	FileName            string    `json:"file_name"`
	MIMEType            string    `json:"mime_type"`
	FileSize            int64     `json:"file_size"`
	OriginalEnabled     bool      `json:"original_enabled"`
	ContentDigest       [32]byte  `json:"-"`
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
	CreateHistoricalInvalidAsset(context.Context, HistoricalInvalidAsset) (HistoricalInvalidAsset, error)
	GetHistoricalInvalidAsset(context.Context, int64) (HistoricalInvalidAsset, error)
}
type InvalidSourceHistoryReader interface {
	GetHistoricalInvalidAsset(context.Context, int64) (HistoricalInvalidAsset, error)
	ListHistoricalInvalidAsset(context.Context, InvalidSourceHistoryQuery) ([]HistoricalInvalidAsset, int64, error)
}
type InvalidSourceHistoryJournal interface {
	LoadInvalidSourceHistory(context.Context, string, string) (InvalidSourceHistoryReceipt, bool, error)
	RecordInvalidSourceHistory(context.Context, InvalidSourceHistoryReceipt) error
}
