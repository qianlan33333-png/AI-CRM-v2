package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrWeComContactHistoryInvalid     = errors.New("invalid WeCom contact history")
	ErrWeComContactHistoryConflict    = errors.New("WeCom contact history conflict")
	ErrWeComContactHistoryUnavailable = errors.New("WeCom contact history unavailable")
)

// Immutable source observations, never current customers, owners or callbacks.
// Source integers keep their signed values; EventTime/CreateTime have unknown units.
// All source/field digests and Follow State are private; target digests cover them.
type HistoricalWeComExternalContactEventLog struct {
	ID                             int64     `json:"id"`
	SourceKeyDigest                [32]byte  `json:"-"`
	SourcePayloadDigest            [32]byte  `json:"-"`
	SourceFieldDigest              [32]byte  `json:"-"`
	SourceID                       int64     `json:"source_id"`
	CorpIDDigest                   [32]byte  `json:"-"`
	EventType                      string    `json:"event_type"`
	ChangeType                     string    `json:"change_type"`
	ExternalUserIDDigest           [32]byte  `json:"-"`
	UserIDDigest                   [32]byte  `json:"-"`
	EventTime                      *int64    `json:"event_time"`
	EventKeyDigest                 [32]byte  `json:"-"`
	PayloadXMLDigest               [32]byte  `json:"-"`
	PayloadJSONDigest              [32]byte  `json:"-"`
	ProcessStatus                  string    `json:"process_status"`
	RetryCount                     int32     `json:"retry_count"`
	ErrorMessageDigest             [32]byte  `json:"-"`
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
	IdentitySyncStatus             string    `json:"identity_sync_status"`
	IdentitySyncErrorCodeDigest    [32]byte  `json:"-"`
	IdentitySyncErrorMessageDigest [32]byte  `json:"-"`
	IdentitySyncResponseDigest     [32]byte  `json:"-"`
}

type HistoricalWeComExternalContactFollowUser struct {
	ID                   int64     `json:"id"`
	SourceKeyDigest      [32]byte  `json:"-"`
	SourcePayloadDigest  [32]byte  `json:"-"`
	SourceFieldDigest    [32]byte  `json:"-"`
	SourceID             int64     `json:"source_id"`
	CorpIDDigest         [32]byte  `json:"-"`
	ExternalUserIDDigest [32]byte  `json:"-"`
	UserIDDigest         [32]byte  `json:"-"`
	RelationStatus       string    `json:"relation_status"`
	IsPrimary            bool      `json:"is_primary"`
	RemarkDigest         [32]byte  `json:"-"`
	DescriptionDigest    [32]byte  `json:"-"`
	AddWay               *int32    `json:"add_way"`
	State                string    `json:"-"`
	OperUserIDDigest     [32]byte  `json:"-"`
	CreateTime           *int64    `json:"create_time"`
	RawFollowUserDigest  [32]byte  `json:"-"`
	FirstSeenAt          time.Time `json:"first_seen_at"`
	LastSeenAt           time.Time `json:"last_seen_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type WeComContactHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type WeComContactHistoryStore interface {
	CreateHistoricalWeComExternalContactEventLog(context.Context, HistoricalWeComExternalContactEventLog) (HistoricalWeComExternalContactEventLog, error)
	GetHistoricalWeComExternalContactEventLog(context.Context, int64) (HistoricalWeComExternalContactEventLog, error)
	CreateHistoricalWeComExternalContactFollowUser(context.Context, HistoricalWeComExternalContactFollowUser) (HistoricalWeComExternalContactFollowUser, error)
	GetHistoricalWeComExternalContactFollowUser(context.Context, int64) (HistoricalWeComExternalContactFollowUser, error)
}
type WeComContactHistoryJournal interface {
	LoadWeComContactHistory(context.Context, string, string) (WeComContactHistoryReceipt, bool, error)
	RecordWeComContactHistory(context.Context, WeComContactHistoryReceipt) error
}
type WeComContactHistoryQuery struct{ Limit, Offset int32 }
type WeComContactHistoryReader interface {
	GetHistoricalWeComExternalContactEventLog(context.Context, int64) (HistoricalWeComExternalContactEventLog, error)
	ListHistoricalWeComExternalContactEventLog(context.Context, WeComContactHistoryQuery) ([]HistoricalWeComExternalContactEventLog, int64, error)
	GetHistoricalWeComExternalContactFollowUser(context.Context, int64) (HistoricalWeComExternalContactFollowUser, error)
	ListHistoricalWeComExternalContactFollowUser(context.Context, WeComContactHistoryQuery) ([]HistoricalWeComExternalContactFollowUser, int64, error)
}
