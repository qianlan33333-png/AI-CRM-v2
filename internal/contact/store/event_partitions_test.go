package store

import (
	"context"
	"errors"
	"testing"
	"time"

	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

type fakeEventPartitionQueries struct {
	calls []contactdb.EnsureCustomerEventPartitionsParams
	err   error
}

func (queries *fakeEventPartitionQueries) EnsureCustomerEventPartitions(
	_ context.Context,
	params contactdb.EnsureCustomerEventPartitionsParams,
) error {
	queries.calls = append(queries.calls, params)
	return queries.err
}

func TestNewEventPartitionMaintainerRejectsNilDependencies(t *testing.T) {
	var typedNil *fakeEventPartitionQueries
	for _, queries := range []eventPartitionQueries{nil, typedNil} {
		maintainer, err := newEventPartitionMaintainer(queries)
		if maintainer != nil || !errors.Is(err, ErrInvalidEventPartitionMaintainer) {
			t.Fatalf("NewEventPartitionMaintainer() = %v, %v; want nil, invalid", maintainer, err)
		}
	}
}

func TestEventPartitionMaintainerValidatesAndNormalizesRequest(t *testing.T) {
	queries := &fakeEventPartitionQueries{}
	maintainer, err := newEventPartitionMaintainer(queries)
	if err != nil {
		t.Fatal(err)
	}
	anchor := time.Date(2026, time.August, 12, 13, 14, 15, 0, time.FixedZone("CST", 8*60*60))
	if err = maintainer.EnsureEventPartitions(context.Background(), anchor, 3); err != nil {
		t.Fatal(err)
	}
	if len(queries.calls) != 1 {
		t.Fatalf("EnsureCustomerEventPartitions calls = %d, want 1", len(queries.calls))
	}
	call := queries.calls[0]
	if !call.Anchor.Valid || !call.Anchor.Time.Equal(anchor.UTC()) ||
		call.Anchor.Time.Location() != time.UTC || call.FutureMonths != 3 {
		t.Fatalf("params = %#v, want UTC anchor and horizon 3", call)
	}
}

func TestEventPartitionMaintainerFailsClosedAndPropagatesDatabaseError(t *testing.T) {
	databaseError := errors.New("partition database failed")
	queries := &fakeEventPartitionQueries{err: databaseError}
	maintainer, err := newEventPartitionMaintainer(queries)
	if err != nil {
		t.Fatal(err)
	}
	validAnchor := time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		ctx     context.Context
		anchor  time.Time
		horizon int32
	}{
		{name: "nil context", anchor: validAnchor, horizon: 3},
		{name: "zero anchor", ctx: context.Background(), horizon: 3},
		{name: "negative horizon", ctx: context.Background(), anchor: validAnchor, horizon: -1},
		{name: "excessive horizon", ctx: context.Background(), anchor: validAnchor, horizon: 37},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := maintainer.EnsureEventPartitions(test.ctx, test.anchor, test.horizon); !errors.Is(err, ErrInvalidEventPartitionMaintainer) {
				t.Fatalf("EnsureEventPartitions() error = %v, want invalid", err)
			}
		})
	}
	if len(queries.calls) != 0 {
		t.Fatalf("invalid requests reached generated query: %d calls", len(queries.calls))
	}
	if err = maintainer.EnsureEventPartitions(context.Background(), validAnchor, 3); !errors.Is(err, databaseError) {
		t.Fatalf("EnsureEventPartitions() error = %v, want database error", err)
	}
}
