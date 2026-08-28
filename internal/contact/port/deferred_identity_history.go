package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDeferredIdentityHistoryInvalid     = errors.New("invalid deferred identity history")
	ErrDeferredIdentityHistoryConflict    = errors.New("deferred identity history conflict")
	ErrDeferredIdentityHistoryUnavailable = errors.New("deferred identity history unavailable")
)

// Unbound historical evidence only. No Customer, identity binding, merge or
// assurance can be created from these facts. Private digests/roots are never API
// fields, but every one must participate in the stored fact digest.
type HistoricalDeferredPerson struct {
	ID                     int64     `json:"id"`
	SourceID               int64     `json:"source_id"`
	SourceKeyDigest        [32]byte  `json:"-"`
	SourcePayloadDigest    [32]byte  `json:"-"`
	SourceFieldDigest      [32]byte  `json:"-"`
	MobileDigest           [32]byte  `json:"-"`
	ThirdPartyUserIDDigest [32]byte  `json:"-"`
	PrivateDigest          [32]byte  `json:"-"`
	RedactedRoots          []string  `json:"-"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type HistoricalDeferredIdentityConflict struct {
	ID                      int64      `json:"id"`
	SourceID                int64      `json:"source_id"`
	SourceKeyDigest         [32]byte   `json:"-"`
	SourcePayloadDigest     [32]byte   `json:"-"`
	SourceFieldDigest       [32]byte   `json:"-"`
	ConflictType            string     `json:"conflict_type"`
	SourceType              string     `json:"source_type"`
	Status                  string     `json:"status"`
	ResolutionStatus        string     `json:"resolution_status"`
	UnionIDDigest           [32]byte   `json:"-"`
	CandidateUnionIDDigest  [32]byte   `json:"-"`
	ExternalUserIDDigest    [32]byte   `json:"-"`
	OpenIDDigest            [32]byte   `json:"-"`
	MobileDigest            [32]byte   `json:"-"`
	LegacySourceKeyDigest   [32]byte   `json:"-"`
	PayloadJSONDigest       [32]byte   `json:"-"`
	SourcePayloadJSONDigest [32]byte   `json:"-"`
	ResolutionNoteDigest    [32]byte   `json:"-"`
	PrivateDigest           [32]byte   `json:"-"`
	RedactedRoots           []string   `json:"-"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	ResolvedAt              *time.Time `json:"resolved_at"`
}

// DM01 provenance is separate from the archive HMAC namespace. The two-row
// missing_customer_root selection must be proven before invoking a writer.
type HistoricalMissingRootIdentity struct {
	ID                       int64     `json:"id"`
	SourceID                 int64     `json:"source_id"`
	SourceKeyDigest          [32]byte  `json:"-"`
	SourcePayloadDigest      [32]byte  `json:"-"`
	SourceFieldDigest        [32]byte  `json:"-"`
	DM01RunID                int64     `json:"-"`
	DM01SourceKeyDigest      [32]byte  `json:"-"`
	DM01SourceHMACKeyVersion string    `json:"-"`
	QuarantineReason         string    `json:"quarantine_reason"`
	Type                     *int32    `json:"type"`
	Status                   string    `json:"status"`
	CorpIDDigest             [32]byte  `json:"-"`
	ExternalUserIDDigest     [32]byte  `json:"-"`
	UnionIDDigest            [32]byte  `json:"-"`
	OpenIDDigest             [32]byte  `json:"-"`
	FollowUserIDDigest       [32]byte  `json:"-"`
	NameDigest               [32]byte  `json:"-"`
	AvatarDigest             [32]byte  `json:"-"`
	GenderDigest             *[32]byte `json:"-"`
	RawProfileDigest         [32]byte  `json:"-"`
	PrivateDigest            [32]byte  `json:"-"`
	RedactedRoots            []string  `json:"-"`
	FirstSeenAt              time.Time `json:"first_seen_at"`
	LastSeenAt               time.Time `json:"last_seen_at"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type DeferredIdentityHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type DeferredIdentityHistoryStore interface {
	CreateHistoricalDeferredPerson(context.Context, HistoricalDeferredPerson) (HistoricalDeferredPerson, error)
	GetHistoricalDeferredPerson(context.Context, int64) (HistoricalDeferredPerson, error)
	CreateHistoricalDeferredIdentityConflict(context.Context, HistoricalDeferredIdentityConflict) (HistoricalDeferredIdentityConflict, error)
	GetHistoricalDeferredIdentityConflict(context.Context, int64) (HistoricalDeferredIdentityConflict, error)
	CreateHistoricalMissingRootIdentity(context.Context, HistoricalMissingRootIdentity) (HistoricalMissingRootIdentity, error)
	GetHistoricalMissingRootIdentity(context.Context, int64) (HistoricalMissingRootIdentity, error)
}

type DeferredIdentityHistoryJournal interface {
	LoadDeferredIdentityHistory(context.Context, string, string) (DeferredIdentityHistoryReceipt, bool, error)
	RecordDeferredIdentityHistory(context.Context, DeferredIdentityHistoryReceipt) error
}

type DeferredIdentityHistoryQuery struct{ Limit, Offset int32 }

type DeferredIdentityHistoryReader interface {
	GetHistoricalDeferredPerson(context.Context, int64) (HistoricalDeferredPerson, error)
	ListHistoricalDeferredPerson(context.Context, DeferredIdentityHistoryQuery) ([]HistoricalDeferredPerson, int64, error)
	GetHistoricalDeferredIdentityConflict(context.Context, int64) (HistoricalDeferredIdentityConflict, error)
	ListHistoricalDeferredIdentityConflict(context.Context, DeferredIdentityHistoryQuery) ([]HistoricalDeferredIdentityConflict, int64, error)
	GetHistoricalMissingRootIdentity(context.Context, int64) (HistoricalMissingRootIdentity, error)
	ListHistoricalMissingRootIdentity(context.Context, DeferredIdentityHistoryQuery) ([]HistoricalMissingRootIdentity, int64, error)
}
