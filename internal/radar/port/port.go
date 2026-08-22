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
