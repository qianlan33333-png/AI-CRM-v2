package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAudienceEditableProjectionRunsOnceInsideTransaction(t *testing.T) {
	at := time.Date(2026, 8, 30, 3, 0, 0, 0, time.UTC)
	order := []string{}
	store := &editableProjectionStoreFake{order: &order, result: AudienceEditableProjectionResult{GroupsCreated: 1, PackagesCreated: 4, MembersProjected: 517, HistoryOnlyPreserved: 34}}
	service := NewAudienceEditableProjectionService(&editableProjectionUOW{order: &order}, store)

	result, err := service.Project(context.Background(), 7, at)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if result != store.result || store.actorID != 7 || !store.at.Equal(at) {
		t.Fatalf("Project() result/store = %#v/%#v", result, store)
	}
	if !reflect.DeepEqual(order, []string{"begin", "project", "commit"}) {
		t.Fatalf("order = %v", order)
	}
}

func TestAudienceEditableProjectionFailsClosed(t *testing.T) {
	at := time.Date(2026, 8, 30, 3, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		service *AudienceEditableProjectionService
		ctx     context.Context
		actorID int64
		at      time.Time
	}{
		{name: "nil service", ctx: context.Background(), actorID: 1, at: at},
		{name: "missing uow", service: NewAudienceEditableProjectionService(nil, &editableProjectionStoreFake{}), ctx: context.Background(), actorID: 1, at: at},
		{name: "missing store", service: NewAudienceEditableProjectionService(&editableProjectionUOW{}, nil), ctx: context.Background(), actorID: 1, at: at},
		{name: "invalid actor", service: NewAudienceEditableProjectionService(&editableProjectionUOW{}, &editableProjectionStoreFake{}), ctx: context.Background(), at: at},
		{name: "non utc", service: NewAudienceEditableProjectionService(&editableProjectionUOW{}, &editableProjectionStoreFake{}), ctx: context.Background(), actorID: 1, at: at.In(time.FixedZone("offset", 3600))},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.service.Project(test.ctx, test.actorID, test.at); !errors.Is(err, ErrAudienceEditableProjection) {
				t.Fatalf("Project() error = %v", err)
			}
		})
	}
}

type editableProjectionUOW struct{ order *[]string }

func (uow *editableProjectionUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow.order != nil {
		*uow.order = append(*uow.order, "begin")
	}
	err := callback(ctx)
	if uow.order != nil {
		if err == nil {
			*uow.order = append(*uow.order, "commit")
		} else {
			*uow.order = append(*uow.order, "rollback")
		}
	}
	return err
}

type editableProjectionStoreFake struct {
	order   *[]string
	result  AudienceEditableProjectionResult
	actorID int64
	at      time.Time
	err     error
}

func (store *editableProjectionStoreFake) ProjectActiveAudienceHistory(_ context.Context, actorID int64, at time.Time) (AudienceEditableProjectionResult, error) {
	if store.order != nil {
		*store.order = append(*store.order, "project")
	}
	store.actorID, store.at = actorID, at
	return store.result, store.err
}
