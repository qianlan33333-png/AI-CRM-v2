package port

import (
	"context"
	"encoding/json"
	"time"
)

// HistoricalHXCMemberUsage preserves a generation observation, not current rights.
type HistoricalHXCMemberUsage struct {
	ID                  int64           `json:"id"`
	SourceKeyDigest     [32]byte        `json:"-"`
	SourcePayloadDigest [32]byte        `json:"-"`
	SourceFieldDigest   [32]byte        `json:"-"`
	Generation          int64           `json:"generation"`
	UnionID             string          `json:"-"`
	OwnerUserID         string          `json:"-"`
	MobileHash          string          `json:"-"`
	IsMember            bool            `json:"is_member"`
	IsRegistered        bool            `json:"is_registered"`
	RegisteredAt        *time.Time      `json:"registered_at"`
	HasRealUsage        bool            `json:"has_real_usage"`
	FirstUsedAt         *time.Time      `json:"first_used_at"`
	LastUsedAt          *time.Time      `json:"last_used_at"`
	MemberSince         *time.Time      `json:"member_since"`
	MembershipExpiresAt *time.Time      `json:"membership_expires_at"`
	MembershipTier      string          `json:"membership_tier"`
	MembershipStatus    string          `json:"membership_status"`
	MembershipSource    string          `json:"membership_source"`
	RegistrationSource  string          `json:"registration_source"`
	UsageSource         string          `json:"usage_source"`
	UpdatedAt           *time.Time      `json:"updated_at"`
	PayloadJSON         json.RawMessage `json:"-"`
	ProjectedAt         time.Time       `json:"projected_at"`
}

const HXCHistoryMemberUsage = "member_usage"

type HXCMemberUsageHistoryQuery struct {
	Generation    *int64
	Limit, Offset int32
}

// Target and the existing HXC receipt journal share one caller transaction.
type HXCMemberUsageHistoryStore interface {
	CreateHistoricalHXCMemberUsage(context.Context, HistoricalHXCMemberUsage) (HistoricalHXCMemberUsage, error)
	GetHistoricalHXCMemberUsage(context.Context, int64) (HistoricalHXCMemberUsage, error)
}

type HXCMemberUsageHistoryReader interface {
	GetHistoricalHXCMemberUsage(context.Context, int64) (HistoricalHXCMemberUsage, error)
	ListHistoricalHXCMemberUsage(context.Context, HXCMemberUsageHistoryQuery) ([]HistoricalHXCMemberUsage, int64, error)
}
