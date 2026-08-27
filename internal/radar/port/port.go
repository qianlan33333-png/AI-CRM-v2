package port

import (
	"context"
	"time"
)

type Application interface {
	List(context.Context, ListInput) (Page, error)
	Get(context.Context, LinkID) (LinkResponse, error)
	Create(context.Context, CreateCommand) (LinkResponse, error)
	Update(context.Context, UpdateCommand) (LinkResponse, error)
	SetStatus(context.Context, SetStatusCommand) (LinkResponse, error)
	Share(context.Context, LinkID) (ShareProjection, error)
	Options(context.Context) Options
}

type Repository interface {
	List(context.Context, ListInput) ([]Link, int64, error)
	Get(context.Context, LinkID) (Link, error)
	GetForUpdate(context.Context, LinkID) (Link, error)
	Create(context.Context, CreateRecord, time.Time) (Link, error)
	Update(context.Context, UpdateRecord, time.Time) (Link, error)
	SetStatus(context.Context, StatusRecord, time.Time) (Link, error)
	ReserveIdempotency(context.Context, ReserveIdempotencyRecord) (IdempotencyRecord, bool, error)
	CompleteIdempotency(context.Context, int64, Link, time.Time) (IdempotencyRecord, error)
}

// HistoricalDraftImporter is the narrow owner-owned write boundary for
// reviewed V1 Radar definitions. It is deliberately separate from normal
// mutations and tracking; implementations must only persist local drafts.
type HistoricalDraftImporter interface {
	ImportHistoricalDraft(context.Context, HistoricalDraftRecord) (Link, bool, error)
}

// ImageReferenceReader is Radar's read-only answer to whether a local link
// currently uses one Media image as its cover. IDs are ordered ascending.
// Media uses this inside its deletion UoW; Radar retains ownership of the
// radar_links projection and its query semantics.
type ImageReferenceReader interface {
	ListImageReferenceLinkIDs(context.Context, int64) ([]int64, error)
}

// AttachmentReferenceReader is Radar's read-only answer to whether a local
// link currently uses one private attachment. IDs are ordered ascending.
type AttachmentReferenceReader interface {
	ListAttachmentReferenceLinkIDs(context.Context, int64) ([]int64, error)
}
