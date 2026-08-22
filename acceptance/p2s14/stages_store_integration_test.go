package p2s14_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestStagesStoreIsTransactionBoundOrderedAndRollbackSafe(t *testing.T) {
	fixture, ctx := openFixture(t)
	createStagesTable(t, ctx, fixture)
	repository := contactstore.NewRepository()
	if _, err := repository.ListStages(ctx); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ListStages() without UoW error = %v, want ErrTransactionRequired", err)
	}

	uow := fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}
	rollback := errors.New("rollback stages fixture")
	var expired context.Context
	err := uow.Within(ctx, func(txCtx context.Context) error {
		expired = txCtx
		late, insertErr := repository.InsertStage(txCtx, contactport.CreateStageCommand{
			Name: "转化", SortOrder: 20, Config: json.RawMessage(`{"color":"amber"}`), Actor: "admin:1",
		})
		if insertErr != nil {
			return insertErr
		}
		early, insertErr := repository.InsertStage(txCtx, contactport.CreateStageCommand{
			Name: "初识", SortOrder: 10, Config: json.RawMessage(`{"color":"blue"}`), Actor: "admin:1",
		})
		if insertErr != nil {
			return insertErr
		}

		stages, listErr := repository.ListStages(txCtx)
		if listErr != nil {
			return listErr
		}
		if len(stages) != 2 || stages[0].ID != early.ID || stages[1].ID != late.ID {
			t.Fatalf("ListStages() order = %#v, want sort_order then id", stages)
		}
		assertJSON(t, stages[0].Config, map[string]any{"color": "blue"})

		renamed, renameErr := repository.RenameStage(txCtx, contactport.RenameStageCommand{
			ID: late.ID, Name: "已转化", Actor: "admin:1",
		})
		if renameErr != nil || renamed.Name != "已转化" || renamed.SortOrder != 20 {
			t.Fatalf("RenameStage() = %#v, %v", renamed, renameErr)
		}
		_, renameErr = repository.RenameStage(txCtx, contactport.RenameStageCommand{
			ID: contactport.StageID(999999), Name: "missing", Actor: "admin:1",
		})
		if !errors.Is(renameErr, contactport.ErrStageNotFound) {
			t.Fatalf("RenameStage() missing error = %v, want ErrStageNotFound", renameErr)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("Within() error = %v, want rollback sentinel", err)
	}
	if _, err = repository.ListStages(expired); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ListStages() expired context error = %v, want ErrTransactionRequired", err)
	}

	err = uow.Within(ctx, func(txCtx context.Context) error {
		stages, listErr := repository.ListStages(txCtx)
		if listErr != nil {
			return listErr
		}
		if len(stages) != 0 {
			t.Fatalf("rollback left stages = %#v", stages)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify rollback: %v", err)
	}
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

func createStagesTable(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.stages (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  config JSONB NOT NULL DEFAULT '{}',
  archived_at TIMESTAMPTZ,
  archived_by TEXT,
  CONSTRAINT stages_archive_pair CHECK (
    (archived_at IS NULL AND archived_by IS NULL)
    OR (archived_at IS NOT NULL AND archived_by IS NOT NULL
        AND btrim(archived_by) = archived_by AND archived_by <> ''
        AND char_length(archived_by) <= 200)
  )
)`)
	if err != nil {
		t.Fatalf("create acceptance stages table: %v", err)
	}
}

func assertJSON(t *testing.T, raw json.RawMessage, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode config %q: %v", raw, err)
	}
	if got["color"] != want["color"] || len(got) != len(want) {
		t.Fatalf("config = %#v, want %#v", got, want)
	}
}
