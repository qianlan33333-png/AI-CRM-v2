package p2s15_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestStageServiceCommitsEventsAndRollsBackWhenAppendFails(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	uow := fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}
	repository := contactstore.NewRepository()
	service := contactapp.NewStageService(uow, repository, eventstore.NewAppender())

	stage, err := service.CreateStage(ctx, contactport.CreateStageCommand{
		Name: "初识", SortOrder: 10, Actor: "admin:1",
	})
	if err != nil {
		t.Fatalf("CreateStage() error = %v", err)
	}
	if string(stage.Config) != "{}" {
		t.Fatalf("CreateStage() config = %s, want {}", stage.Config)
	}
	stages, err := service.ListStages(ctx)
	if err != nil || len(stages) != 1 || stages[0].ID != stage.ID {
		t.Fatalf("ListStages() = %#v, %v", stages, err)
	}
	assertEvent(t, ctx, fixture, "stage.created", stage.ID, "初识", "admin:1", 1)

	renamed, err := service.RenameStage(ctx, contactport.RenameStageCommand{
		ID: stage.ID, Name: "已联系", Actor: "admin:2",
	})
	if err != nil || renamed.Name != "已联系" {
		t.Fatalf("RenameStage() = %#v, %v", renamed, err)
	}
	assertEvent(t, ctx, fixture, "stage.renamed", stage.ID, "已联系", "admin:2", 2)

	sentinel := errors.New("append failed")
	failingService := contactapp.NewStageService(uow, repository, failingAppender{err: sentinel})
	created, err := failingService.CreateStage(ctx, contactport.CreateStageCommand{
		Name: "不得提交", SortOrder: 20, Config: json.RawMessage(`{}`), Actor: "admin:3",
	})
	if !errors.Is(err, sentinel) || !zeroStage(created) {
		t.Fatalf("CreateStage() on event failure = %#v, %v", created, err)
	}
	assertCountsAndName(t, ctx, fixture, 1, 2, stage.ID, "已联系")

	renamed, err = failingService.RenameStage(ctx, contactport.RenameStageCommand{
		ID: stage.ID, Name: "不得改名", Actor: "admin:3",
	})
	if !errors.Is(err, sentinel) || !zeroStage(renamed) {
		t.Fatalf("RenameStage() on event failure = %#v, %v", renamed, err)
	}
	assertCountsAndName(t, ctx, fixture, 1, 2, stage.ID, "已联系")
}

func zeroStage(stage contactport.Stage) bool {
	return stage.ID == 0 && stage.Name == "" && stage.SortOrder == 0 && len(stage.Config) == 0
}

type fixtureUoW struct{ delegate platformport.UnitOfWork }

func (uow fixtureUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return uow.delegate.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `SET LOCAL search_path TO acceptance_fixtures, public`); err != nil {
			return err
		}
		return callback(txCtx)
	})
}

type failingAppender struct{ err error }

func (appender failingAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, appender.err
}

func openFixture(t *testing.T) (*acceptancefixtures.PostgreSQL, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgreSQL() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})
	return fixture, ctx
}

func createTables(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.stages (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'
);
CREATE TABLE acceptance_fixtures.event_log (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type TEXT NOT NULL,
  customer_id BIGINT,
  payload JSONB NOT NULL DEFAULT '{}',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  idempotency_key TEXT NOT NULL UNIQUE,
  dispatched BOOLEAN NOT NULL DEFAULT FALSE
)`)
	if err != nil {
		t.Fatalf("create acceptance tables: %v", err)
	}
}

func assertEvent(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
	eventType string,
	stageID contactport.StageID,
	name string,
	actor string,
	wantCount int,
) {
	t.Helper()
	var gotType, key string
	var payload []byte
	var occurredAt time.Time
	var customerID pgtype.Int8
	err := fixture.Pool().QueryRow(ctx, `
SELECT event_type, customer_id, payload, occurred_at, idempotency_key
FROM acceptance_fixtures.event_log
ORDER BY id DESC LIMIT 1`).Scan(&gotType, &customerID, &payload, &occurredAt, &key)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	var decoded struct {
		StageID contactport.StageID `json:"stage_id"`
		Name    string              `json:"name"`
		Actor   string              `json:"actor"`
	}
	if err = json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	if gotType != eventType || customerID.Valid || decoded.StageID != stageID ||
		decoded.Name != name || decoded.Actor != actor || occurredAt.IsZero() ||
		len(key) != len(eventType)+1+32 || key[:len(eventType)+1] != eventType+":" {
		t.Fatalf("event = type:%q customer:%#v payload:%s at:%v key:%q", gotType, customerID, payload, occurredAt, key)
	}
	assertCountsAndName(t, ctx, fixture, 1, wantCount, stageID, name)
}

func assertCountsAndName(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
	wantStages int,
	wantEvents int,
	stageID contactport.StageID,
	wantName string,
) {
	t.Helper()
	var stageCount, eventCount int
	var name string
	err := fixture.Pool().QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM acceptance_fixtures.stages),
  (SELECT count(*) FROM acceptance_fixtures.event_log),
  (SELECT name FROM acceptance_fixtures.stages WHERE id = $1)
`, int64(stageID)).Scan(&stageCount, &eventCount, &name)
	if err != nil {
		t.Fatalf("query committed facts: %v", err)
	}
	if stageCount != wantStages || eventCount != wantEvents || name != wantName {
		t.Fatalf("committed facts = stages:%d events:%d name:%q", stageCount, eventCount, name)
	}
}
