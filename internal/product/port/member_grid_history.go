package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMemberGridHistoryInvalid     = errors.New("invalid member grid history")
	ErrMemberGridHistoryConflict    = errors.New("member grid history conflict")
	ErrMemberGridHistoryUnavailable = errors.New("member grid history unavailable")
)

// Historical views do not become executable V2 saved views. Config stays in the
// sealed source archive; ProductID is set only by a verified Product crosswalk.
type HistoricalMemberView struct {
	ID                     int64     `json:"id"`
	SourceKeyDigest        [32]byte  `json:"source_key_digest"`
	SourceViewID           int64     `json:"source_view_id"`
	SourceServiceProductID int64     `json:"source_service_product_id"`
	ProductID              *int64    `json:"product_id"`
	Name                   string    `json:"name"`
	Position               int64     `json:"position"`
	IsDefault              bool      `json:"is_default"`
	SchemaVersion          int16     `json:"schema_version"`
	ConfigDigest           [32]byte  `json:"config_digest"`
	Version                int64     `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	SourcePayloadDigest    [32]byte  `json:"source_payload_digest"`
}

// All usage and login flags are source-time facts, never current entitlement or
// authentication. Raw user identifiers remain in the sealed source archive.
type HistoricalMemberUsage struct {
	ID                  int64      `json:"id"`
	SourceKeyDigest     [32]byte   `json:"source_key_digest"`
	CustomerID          *int64     `json:"customer_id"`
	FormallyLoggedIn    bool       `json:"formally_logged_in"`
	HasTokenUsage       bool       `json:"has_token_usage"`
	LearningPlanID      string     `json:"learning_plan_id"`
	LearningPlanCurrent *int64     `json:"learning_plan_current"`
	LearningPlanTotal   *int64     `json:"learning_plan_total"`
	OpenCount7D         int64      `json:"open_count_7d"`
	LastOpenAt          *time.Time `json:"last_open_at"`
	RefreshedAt         time.Time  `json:"refreshed_at"`
	SourcePayloadDigest [32]byte   `json:"source_payload_digest"`
	RecoveryEntryDigest [32]byte   `json:"recovery_entry_digest"`
}

const (
	MemberGridHistoryView  = "member_view"
	MemberGridHistoryUsage = "member_usage"
)

type MemberGridHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

// Store and journal use the same caller transaction and emit no runtime event.
type MemberGridHistoryStore interface {
	CreateHistoricalMemberView(context.Context, HistoricalMemberView) (HistoricalMemberView, error)
	GetHistoricalMemberView(context.Context, int64) (HistoricalMemberView, error)
	CreateHistoricalMemberUsage(context.Context, HistoricalMemberUsage) (HistoricalMemberUsage, error)
	GetHistoricalMemberUsage(context.Context, int64) (HistoricalMemberUsage, error)
}

type MemberGridHistoryJournal interface {
	LoadMemberGridHistory(context.Context, string, string) (MemberGridHistoryReceipt, bool, error)
	RecordMemberGridHistory(context.Context, MemberGridHistoryReceipt) error
}

type MemberGridHistoryQuery struct {
	ProductID     *int64
	CustomerID    *int64
	Limit, Offset int32
}

type MemberGridHistoryReader interface {
	GetHistoricalMemberView(context.Context, int64) (HistoricalMemberView, error)
	ListHistoricalMemberViews(context.Context, MemberGridHistoryQuery) ([]HistoricalMemberView, int64, error)
	GetHistoricalMemberUsage(context.Context, int64) (HistoricalMemberUsage, error)
	ListHistoricalMemberUsage(context.Context, MemberGridHistoryQuery) ([]HistoricalMemberUsage, int64, error)
}
