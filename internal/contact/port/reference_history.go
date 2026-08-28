package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrReferenceHistoryInvalid     = errors.New("invalid contact reference history")
	ErrReferenceHistoryConflict    = errors.New("contact reference history conflict")
	ErrReferenceHistoryUnavailable = errors.New("contact reference history unavailable")
)

// These are historical source facts. Optional target references never grant
// access, reassign an owner, bind a customer or upgrade identity assurance.
type HistoricalExternalContactBinding struct {
	ID                       int64     `json:"id"`
	SourceKeyDigest          [32]byte  `json:"-"`
	SourcePayloadDigest      [32]byte  `json:"-"`
	SourceFieldDigest        [32]byte  `json:"-"`
	ExternalUserIDDigest     [32]byte  `json:"-"`
	SourcePersonID           int64     `json:"source_person_id"`
	PersonHistoryID          *int64    `json:"person_history_id"`
	IdentityID               *int64    `json:"identity_id"`
	IdentityAssurance        string    `json:"identity_assurance"`
	FirstBoundByUserIDDigest [32]byte  `json:"-"`
	FirstOwnerUserIDDigest   [32]byte  `json:"-"`
	LastOwnerUserIDDigest    [32]byte  `json:"-"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type HistoricalWeComDirectoryMember struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	SourceID            int64     `json:"source_id"`
	WeComCorpIDDigest   [32]byte  `json:"-"`
	CorpIDDigest        [32]byte  `json:"-"`
	WeComUserIDDigest   [32]byte  `json:"-"`
	CorpAttribution     string    `json:"corp_attribution"`
	MatchedStaffID      *int64    `json:"matched_staff_id"`
	DisplayName         string    `json:"display_name"`
	DepartmentIDsDigest [32]byte  `json:"-"`
	DepartmentName      string    `json:"department_name"`
	Position            string    `json:"position"`
	WeComStatus         *int32    `json:"wecom_status"`
	IsActive            bool      `json:"is_active"`
	SyncedAt            time.Time `json:"synced_at"`
	RawPayloadDigest    [32]byte  `json:"-"`
	MobileDigest        [32]byte  `json:"-"`
	AvatarURLDigest     [32]byte  `json:"-"`
	UpdatedByDigest     [32]byte  `json:"-"`
	FirstSeenAt         time.Time `json:"first_seen_at"`
	LastSyncedAt        time.Time `json:"last_synced_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ReferenceHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type ReferenceHistoryStore interface {
	CreateHistoricalExternalContactBinding(context.Context, HistoricalExternalContactBinding) (HistoricalExternalContactBinding, error)
	GetHistoricalExternalContactBinding(context.Context, int64) (HistoricalExternalContactBinding, error)
	CreateHistoricalWeComDirectoryMember(context.Context, HistoricalWeComDirectoryMember) (HistoricalWeComDirectoryMember, error)
	GetHistoricalWeComDirectoryMember(context.Context, int64) (HistoricalWeComDirectoryMember, error)
}

type ReferenceHistoryJournal interface {
	LoadReferenceHistory(context.Context, string, string) (ReferenceHistoryReceipt, bool, error)
	RecordReferenceHistory(context.Context, ReferenceHistoryReceipt) error
}

type ReferenceHistoryQuery struct{ Limit, Offset int32 }

type ReferenceHistoryReader interface {
	GetHistoricalExternalContactBinding(context.Context, int64) (HistoricalExternalContactBinding, error)
	ListHistoricalExternalContactBinding(context.Context, ReferenceHistoryQuery) ([]HistoricalExternalContactBinding, int64, error)
	GetHistoricalWeComDirectoryMember(context.Context, int64) (HistoricalWeComDirectoryMember, error)
	ListHistoricalWeComDirectoryMember(context.Context, ReferenceHistoryQuery) ([]HistoricalWeComDirectoryMember, int64, error)
}
