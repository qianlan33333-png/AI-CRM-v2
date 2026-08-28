package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCustomerStateHistoryInvalid     = errors.New("invalid customer state history")
	ErrCustomerStateHistoryConflict    = errors.New("customer state history conflict")
	ErrCustomerStateHistoryUnavailable = errors.New("customer state history unavailable")
)

// These are immutable observations, never live customer status or WeCom tags.
// Private source fields are retained for traceability but must not enter API JSON.
// Target digests must cover every field including the private fields.

type HistoricalCustomerStatusSnapshot struct {
	ID                    int64     `json:"id"`
	SourceKeyDigest       [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest   [32]byte  `json:"source_payload_digest"`
	SourceFieldDigest     [32]byte  `json:"source_field_digest"`
	SignupStatus          string    `json:"signup_status"`
	SignupLabelName       string    `json:"signup_label_name"`
	CustomerNameSnapshot  string    `json:"-"`
	OwnerUserIDSnapshot   string    `json:"-"`
	SetByUserIDDigest     [32]byte  `json:"set_by_userid_digest"`
	SetAt                 time.Time `json:"set_at"`
	WeComTagSyncStatus    string    `json:"wecom_tag_sync_status"`
	WeComTagSyncErrorHash [32]byte  `json:"wecom_tag_sync_error_hash"`
	StatusFlagsDigest     [32]byte  `json:"status_flags_digest"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	UnionID               string    `json:"-"`
}

type HistoricalCustomerStatusChange struct {
	ID                    int64     `json:"id"`
	SourceKeyDigest       [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest   [32]byte  `json:"source_payload_digest"`
	SourceFieldDigest     [32]byte  `json:"source_field_digest"`
	SourceID              int64     `json:"source_id"`
	OldSignupStatus       string    `json:"old_signup_status"`
	NewSignupStatus       string    `json:"new_signup_status"`
	OldLabelName          string    `json:"old_label_name"`
	NewLabelName          string    `json:"new_label_name"`
	CustomerNameSnapshot  string    `json:"-"`
	OwnerUserIDSnapshot   string    `json:"-"`
	SetByUserIDDigest     [32]byte  `json:"set_by_userid_digest"`
	SetAt                 time.Time `json:"set_at"`
	WeComTagSyncStatus    string    `json:"wecom_tag_sync_status"`
	WeComTagSyncErrorHash [32]byte  `json:"wecom_tag_sync_error_hash"`
	StatusFlagsDigest     [32]byte  `json:"status_flags_digest"`
	CreatedAt             time.Time `json:"created_at"`
	UnionID               string    `json:"-"`
}

type HistoricalClassTermTagMapping struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
	SourceFieldDigest   [32]byte  `json:"source_field_digest"`
	SourceID            int64     `json:"source_id"`
	TagGroupName        string    `json:"tag_group_name"`
	TagName             string    `json:"tag_name"`
	ClassTermNo         int32     `json:"class_term_no"`
	ClassTermLabel      string    `json:"class_term_label"`
	OriginalActive      bool      `json:"original_active"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	StrategySourceID    string    `json:"-"`
	GroupSourceID       string    `json:"-"`
	TagSourceID         string    `json:"-"`
}

type CustomerStateHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type CustomerStateHistoryStore interface {
	CreateHistoricalCustomerStatusSnapshot(context.Context, HistoricalCustomerStatusSnapshot) (HistoricalCustomerStatusSnapshot, error)
	GetHistoricalCustomerStatusSnapshot(context.Context, int64) (HistoricalCustomerStatusSnapshot, error)
	CreateHistoricalCustomerStatusChange(context.Context, HistoricalCustomerStatusChange) (HistoricalCustomerStatusChange, error)
	GetHistoricalCustomerStatusChange(context.Context, int64) (HistoricalCustomerStatusChange, error)
	CreateHistoricalClassTermTagMapping(context.Context, HistoricalClassTermTagMapping) (HistoricalClassTermTagMapping, error)
	GetHistoricalClassTermTagMapping(context.Context, int64) (HistoricalClassTermTagMapping, error)
}

type CustomerStateHistoryJournal interface {
	LoadCustomerStateHistory(context.Context, string, string) (CustomerStateHistoryReceipt, bool, error)
	RecordCustomerStateHistory(context.Context, CustomerStateHistoryReceipt) error
}

type CustomerStateHistoryQuery struct{ Limit, Offset int32 }

type CustomerStateHistoryReader interface {
	GetHistoricalCustomerStatusSnapshot(context.Context, int64) (HistoricalCustomerStatusSnapshot, error)
	ListHistoricalCustomerStatusSnapshot(context.Context, CustomerStateHistoryQuery) ([]HistoricalCustomerStatusSnapshot, int64, error)
	GetHistoricalCustomerStatusChange(context.Context, int64) (HistoricalCustomerStatusChange, error)
	ListHistoricalCustomerStatusChange(context.Context, CustomerStateHistoryQuery) ([]HistoricalCustomerStatusChange, int64, error)
	GetHistoricalClassTermTagMapping(context.Context, int64) (HistoricalClassTermTagMapping, error)
	ListHistoricalClassTermTagMapping(context.Context, CustomerStateHistoryQuery) ([]HistoricalClassTermTagMapping, int64, error)
}
