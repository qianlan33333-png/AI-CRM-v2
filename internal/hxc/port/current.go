package port

import (
	"context"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type MatchState string

const (
	MatchStateMatched   MatchState = "matched"
	MatchStateUnmatched MatchState = "unmatched"
	MatchStateConflict  MatchState = "conflict"
)

type CapabilityUsage struct {
	Count7D    int64      `json:"count_7d"`
	Count30D   int64      `json:"count_30d"`
	CountTotal int64      `json:"count_total"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type SourceCurrent struct {
	HXCUserID             string
	UnionID               string
	Phone                 string
	SubscriptionTier      string
	SubscriptionExpiresAt *time.Time
	MonthlyChatQuota      int32
	CurrentPeriodUsed     int32
	ConsultationLimit     int32
	ConsultationUsed      int32
	Sessions7D            int64
	Sessions30D           int64
	SessionsTotal         int64
	UserMessages7D        int64
	UserMessages30D       int64
	UserMessagesTotal     int64
	CapabilityUsage       map[string]CapabilityUsage
	LastUsedAt            *time.Time
	LastCapability        *string
	BusinessStage         *string
	MainLineType          *string
	UserSegment           *string
	FocusTopics           []string
	PainTag               *string
	SourceUpdatedAt       time.Time
}

type Current struct {
	SourceCurrent
	CustomerID contactport.CustomerID
	MatchState MatchState
	SyncedAt   time.Time
}

type SyncSummary struct {
	SourceCount    int32
	MatchedCount   int32
	UnmatchedCount int32
	ConflictCount  int32
}

type CurrentSnapshot struct {
	Found        bool
	Current      Current
	LastSyncedAt *time.Time
}

type CurrentReader interface {
	ReadCustomerCurrent(context.Context, contactport.CustomerID) (CurrentSnapshot, error)
}
