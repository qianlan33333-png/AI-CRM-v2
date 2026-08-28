package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRadarClickHistoryInvalid     = errors.New("invalid radar history")
	ErrRadarClickHistoryConflict    = errors.New("radar history conflict")
	ErrRadarClickHistoryUnavailable = errors.New("radar history unavailable")
)

// Immutable source history. Source IDs are not current V2 IDs; private digests
// bind all original fields without exposing identities or replaying execution.
type HistoricalRadarClick struct {
	ID                     int64     `json:"id"`
	SourceKeyDigest        [32]byte  `json:"-"`
	SourcePayloadDigest    [32]byte  `json:"-"`
	SourceFieldDigest      [32]byte  `json:"-"`
	SourceID               int64     `json:"source_id"`
	LinkSourceID           int64     `json:"link_source_id"`
	RadarLinkID            *int64    `json:"radar_link_id"`
	CustomerID             *int64    `json:"customer_id"`
	Code                   string    `json:"code"`
	RawStage               string    `json:"raw_stage"`
	SourceChannel          string    `json:"source_channel"`
	TargetTypeSnapshot     string    `json:"target_type_snapshot"`
	SourceChannelSnapshot  string    `json:"source_channel_snapshot"`
	ErrorCode              string    `json:"error_code"`
	CreatedAt              time.Time `json:"created_at"`
	OpenIDDigest           [32]byte  `json:"-"`
	UnionIDDigest          [32]byte  `json:"-"`
	ExternalUserIDDigest   [32]byte  `json:"-"`
	CampaignIDDigest       [32]byte  `json:"-"`
	StaffIDDigest          [32]byte  `json:"-"`
	UserAgentDigest        [32]byte  `json:"-"`
	IPDigest               [32]byte  `json:"-"`
	PersonIDDigest         [32]byte  `json:"-"`
	IPHashDigest           [32]byte  `json:"-"`
	CampaignSnapshotDigest [32]byte  `json:"-"`
	StaffSnapshotDigest    [32]byte  `json:"-"`
	RefererDigest          [32]byte  `json:"-"`
	QueryParamsDigest      [32]byte  `json:"-"`
}

type RadarClickHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type RadarClickHistoryQuery struct{ Limit, Offset int32 }
type RadarClickHistoryStore interface {
	CreateHistoricalRadarClick(context.Context, HistoricalRadarClick) (HistoricalRadarClick, error)
	GetHistoricalRadarClick(context.Context, int64) (HistoricalRadarClick, error)
}
type RadarClickHistoryJournal interface {
	LoadRadarClickHistory(context.Context, string, string) (RadarClickHistoryReceipt, bool, error)
	RecordRadarClickHistory(context.Context, RadarClickHistoryReceipt) error
}
type RadarClickHistoryReader interface {
	GetHistoricalRadarClick(context.Context, int64) (HistoricalRadarClick, error)
	ListHistoricalRadarClick(context.Context, RadarClickHistoryQuery) ([]HistoricalRadarClick, int64, error)
}
