package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

const maximumEventPartitionHorizon int32 = 36

var ErrInvalidEventPartitionMaintainer = errors.New("invalid customer event partition maintainer")

type eventPartitionQueries interface {
	EnsureCustomerEventPartitions(context.Context, contactdb.EnsureCustomerEventPartitionsParams) error
}

type EventPartitionMaintainer struct {
	queries eventPartitionQueries
}

func NewEventPartitionMaintainer(database contactdb.DBTX) (*EventPartitionMaintainer, error) {
	if isNilPartitionDependency(database) {
		return nil, ErrInvalidEventPartitionMaintainer
	}
	return newEventPartitionMaintainer(contactdb.New(database))
}

func newEventPartitionMaintainer(queries eventPartitionQueries) (*EventPartitionMaintainer, error) {
	if isNilPartitionDependency(queries) {
		return nil, ErrInvalidEventPartitionMaintainer
	}
	return &EventPartitionMaintainer{queries: queries}, nil
}

func (maintainer *EventPartitionMaintainer) EnsureEventPartitions(
	ctx context.Context,
	anchor time.Time,
	futureMonths int32,
) error {
	if maintainer == nil || isNilPartitionDependency(maintainer.queries) || ctx == nil ||
		anchor.IsZero() || futureMonths < 0 || futureMonths > maximumEventPartitionHorizon {
		return ErrInvalidEventPartitionMaintainer
	}
	return maintainer.queries.EnsureCustomerEventPartitions(ctx, contactdb.EnsureCustomerEventPartitionsParams{
		Anchor:       pgtype.Timestamptz{Time: anchor.UTC(), Valid: true},
		FutureMonths: futureMonths,
	})
}

func isNilPartitionDependency(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}
