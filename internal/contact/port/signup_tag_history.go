package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSignupTagHistoryInvalid     = errors.New("invalid signup tag history")
	ErrSignupTagHistoryConflict    = errors.New("signup tag history conflict")
	ErrSignupTagHistoryUnavailable = errors.New("signup tag history unavailable")
)

// TagSourceID belongs only to the sealed V1 namespace, never the current tag
// catalogue. Reading/importing this fact must not enable a rule or sync tags.
type HistoricalSignupTagRule struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
	TagSourceID         string    `json:"tag_source_id"`
	TagName             string    `json:"tag_name"`
	SignupStatus        string    `json:"signup_status"`
	OriginalActive      bool      `json:"original_active"`
	UpdatedAt           time.Time `json:"updated_at"`
}
type SignupTagHistoryReceipt struct {
	SourceIdentifier            string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type SignupTagHistoryStore interface {
	CreateHistoricalSignupTagRule(context.Context, HistoricalSignupTagRule) (HistoricalSignupTagRule, error)
	GetHistoricalSignupTagRule(context.Context, int64) (HistoricalSignupTagRule, error)
}
type SignupTagHistoryJournal interface {
	LoadSignupTagHistory(context.Context, string) (SignupTagHistoryReceipt, bool, error)
	RecordSignupTagHistory(context.Context, SignupTagHistoryReceipt) error
}
type SignupTagHistoryReader interface {
	GetHistoricalSignupTagRule(context.Context, int64) (HistoricalSignupTagRule, error)
	ListHistoricalSignupTagRules(context.Context, int32, int32) ([]HistoricalSignupTagRule, int64, error)
}
