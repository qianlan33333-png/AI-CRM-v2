package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerport "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

type tagProviderFunc func(context.Context, tag.ProviderCommand, eerport.Attempt) (tag.ProviderResult, error)

func (function tagProviderFunc) Execute(ctx context.Context, command tag.ProviderCommand, attempt eerport.Attempt) (tag.ProviderResult, error) {
	return function(ctx, command, attempt)
}

func TestB1WC01WeComTagEffectPG16(t *testing.T) {
	dsn := os.Getenv("P4B1WC01_WECOM_TAG_EFFECT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("P4B1WC01_WECOM_TAG_EFFECT_TEST_DATABASE_URL is not configured")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(dsn, acceptancefixtures.B1WC01WeComTagEffectDatabaseName); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL server_version_num=%q err=%v, want 160014", version, err)
	}
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}

	uow := platformstore.NewUnitOfWork(pool)
	runtimeStore := eerstore.NewRepository(pool, uow)
	runtime, err := eer.NewService(runtimeStore)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := tag.NewRiverJobInserter(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := tag.NewService(uow, wecomstore.NewTagEffectRepository(pool), runtime, jobs, "corp-wc01")
	if err != nil {
		t.Fatal(err)
	}
	legacyRepository, err := contactstore.NewLegacyTagExecutionRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	legacyLive := contactapp.NewLegacyTagLiveMutationService(uow, legacyRepository, eventstore.NewAppender(), legacyRepository)
	legacySync := contactapp.NewLegacyTagSyncService(uow, legacyRepository, eventstore.NewAppender(), legacyRepository)

	assertLegacyRowsWereNotReplayed(t, ctx, pool)
	t.Run("historical queued receipt replay cannot create a typed effect", func(t *testing.T) {
		historicalFixture := "wc01-preexisting-legacy"
		_, err := legacyLive.RequestWithCommitHook(ctx, contactapp.LegacyTagLiveMutationCommand{
			Actor: 88, IdempotencyKey: historicalFixture, TraceID: "wc01-preexisting",
			Operation: contactapp.LegacyTagLiveMutationMark, Payload: json.RawMessage(`{}`),
		}, func(txCtx context.Context, acceptance contactapp.LegacyTagLiveMutationAcceptance, replay bool) error {
			if !replay {
				return errors.New("historical receipt was not classified as replay")
			}
			_, replayErr := service.ReplayInTransaction(txCtx, tag.QueueCommand{
				LegacyReceiptID: acceptance.ReceiptID, Actor: 88, IdempotencyKey: historicalFixture,
				Operation: tag.OperationMark, ExternalUserID: "external-preexisting", ProviderTagIDs: []string{"tag-preexisting"},
			})
			return replayErr
		})
		if !errors.Is(err, tag.ErrEffectUnavailable) {
			t.Fatalf("historical replay error = %v, want unavailable", err)
		}
		assertLegacyRowsWereNotReplayed(t, ctx, pool)
	})
	t.Run("typed queue failure rolls back the legacy acceptance", func(t *testing.T) {
		_, err := legacyLive.RequestWithCommitHook(ctx, contactapp.LegacyTagLiveMutationCommand{
			Actor: 100, IdempotencyKey: "wc01-atomic-rollback-0001", TraceID: "wc01-atomic-rollback",
			Operation: contactapp.LegacyTagLiveMutationMark, Payload: json.RawMessage(`{"source":"wc01-atomic-rollback"}`),
		}, func(txCtx context.Context, acceptance contactapp.LegacyTagLiveMutationAcceptance, _ bool) error {
			_, queueErr := service.QueueInTransaction(txCtx, tag.QueueCommand{
				LegacyReceiptID: acceptance.ReceiptID, Actor: 100, IdempotencyKey: "wc01-atomic-rollback-0001",
				Operation: tag.OperationMark,
			})
			return queueErr
		})
		if !errors.Is(err, tag.ErrInvalidCommand) {
			t.Fatalf("atomic rollback error = %v, want ErrInvalidCommand", err)
		}
		var receipts, legacyJobs int
		if err = pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM public.legacy_tag_live_mutation_receipts WHERE trace_id='wc01-atomic-rollback'),
			(SELECT count(*) FROM river_job WHERE kind=$1)`, contactapp.LegacyTagLiveMutationJobKind).Scan(&receipts, &legacyJobs); err != nil {
			t.Fatal(err)
		}
		if receipts != 0 || legacyJobs != 0 {
			t.Fatalf("rolled-back legacy receipts/jobs = %d/%d, want 0/0", receipts, legacyJobs)
		}
	})
	t.Run("disabled provider is durable final failure and queue replay is local", func(t *testing.T) {
		command := tag.QueueCommand{
			Actor: 101, IdempotencyKey: "wc01-disabled-queue-0001",
			Operation: tag.OperationMark, ExternalUserID: "external-disabled", ProviderTagIDs: []string{"tag-b", "tag-a"},
		}
		first, receiptID := requestAtomicLegacyMutation(t, ctx, legacyLive, service, command)
		command.LegacyReceiptID = receiptID
		replay, err := service.Queue(ctx, command)
		if err != nil || replay != first {
			t.Fatalf("queue replay = %#v, %v; first=%#v", replay, err, first)
		}
		assertTagJobCount(t, ctx, pool, 1)
		execution, err := service.Execute(ctx, first.EffectID, testDigest("disabled-worker"), tag.DisabledProvider{})
		if err != nil || execution.State != eerport.StateFinalFailed || execution.RealExternalCallExecuted {
			t.Fatalf("disabled execution = %#v, %v", execution, err)
		}
	})

	t.Run("transport unknown is manually reconciled and never replayed", func(t *testing.T) {
		accepted, _ := requestAtomicLegacyMutation(t, ctx, legacyLive, service, tag.QueueCommand{
			Actor: 102, IdempotencyKey: "wc01-unknown-queue-0001",
			Operation: tag.OperationUnmark, ExternalUserID: "external-unknown", ProviderTagIDs: []string{"tag-unknown"},
		})
		calls := 0
		provider := tagProviderFunc(func(context.Context, tag.ProviderCommand, eerport.Attempt) (tag.ProviderResult, error) {
			calls++
			return tag.ProviderResult{}, errors.New("transport result is unknown")
		})
		first, err := service.Execute(ctx, accepted.EffectID, testDigest("unknown-worker"), provider)
		if err != nil || first.State != eerport.StateOutcomeUnknown || !first.ManualReconcileRequired || calls != 1 {
			t.Fatalf("unknown execution = %#v, calls=%d, err=%v", first, calls, err)
		}
		replay, err := service.Execute(ctx, accepted.EffectID, testDigest("unknown-worker-replay"), provider)
		if err != nil || replay.State != eerport.StateOutcomeUnknown || replay.ProviderCallAttempted || calls != 1 {
			t.Fatalf("unknown replay = %#v, calls=%d, err=%v", replay, calls, err)
		}
		record, err := wecomstore.NewTagEffectRepository(pool).Get(ctx, accepted.EffectID)
		if err != nil {
			t.Fatal(err)
		}
		reconciled, err := service.Reconcile(ctx, tag.ReconcileCommand{
			EffectID: accepted.EffectID, Actor: 902, IdempotencyKey: "wc01-manual-reconcile-0001",
			Generation: record.Generation, Fence: record.Fence, LeaseExpiresAt: record.LeaseExpiresAt,
			EvidenceDigest: testDigest("manual-provider-evidence"), Resolution: tag.ResolutionProviderNotApplied,
		})
		if err != nil || reconciled.State != eerport.StateReconciled || reconciled.ProviderCallAttempted || reconciled.ProviderSuccessClaimed {
			t.Fatalf("manual reconcile = %#v, %v", reconciled, err)
		}
	})

	t.Run("catalog snapshots are immutable observations", func(t *testing.T) {
		accepted, _ := requestAtomicLegacySync(t, ctx, legacySync, service, tag.QueueCommand{
			Actor: 103, IdempotencyKey: "wc01-catalog-queue-0001",
			Operation: tag.OperationCatalogSync, SyncTrigger: tag.SyncTriggerManual,
		})
		calls := 0
		provider := tagProviderFunc(func(context.Context, tag.ProviderCommand, eerport.Attempt) (tag.ProviderResult, error) {
			calls++
			return tag.ProviderResult{Completion: "executed", ReceiptDigest: testDigest("catalog-provider-receipt"), Catalog: tag.CatalogSnapshot{
				Observed: true,
				Groups:   []tag.CatalogGroup{{ProviderGroupID: "group-1", Name: "Lifecycle", Order: 1}},
				Tags:     []tag.CatalogTag{{ProviderTagID: "tag-1", ProviderGroupID: "group-1", Name: "Warm", Order: 1}},
			}}, nil
		})
		execution, err := service.Execute(ctx, accepted.EffectID, testDigest("catalog-worker"), provider)
		if err != nil || execution.State != eerport.StateExecuted || calls != 1 {
			t.Fatalf("catalog execution = %#v, calls=%d, err=%v", execution, calls, err)
		}
		replay, err := service.Execute(ctx, accepted.EffectID, testDigest("catalog-worker-replay"), provider)
		if err != nil || replay.ProviderCallAttempted || calls != 1 {
			t.Fatalf("catalog replay = %#v, calls=%d, err=%v", replay, calls, err)
		}
		var snapshots, groups, tags int
		if err = pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM public.wecom_tag_catalog_snapshots),
			(SELECT count(*) FROM public.wecom_tag_catalog_groups),
			(SELECT count(*) FROM public.wecom_tag_catalog_tags)`).Scan(&snapshots, &groups, &tags); err != nil {
			t.Fatal(err)
		}
		if snapshots != 1 || groups != 1 || tags != 1 {
			t.Fatalf("catalog snapshot/groups/tags = %d/%d/%d, want 1/1/1", snapshots, groups, tags)
		}
		_, err = pool.Exec(ctx, `UPDATE public.wecom_tag_catalog_snapshots SET corp_id='overwrite'`)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
			t.Fatalf("catalog overwrite error = %v, want SQLSTATE 55000", err)
		}
	})
	assertTagJobCount(t, ctx, pool, 3)
}

func assertLegacyRowsWereNotReplayed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var legacy, typed, effects, jobs int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM public.legacy_tag_live_mutation_receipts WHERE trace_id='wc01-preexisting'),
		(SELECT count(*) FROM public.wecom_tag_effects),
		(SELECT count(*) FROM public.external_effects WHERE owner='wecom' AND kind='wecom_tag_sync'),
		(SELECT count(*) FROM river_job WHERE kind=$1)`, tag.JobKind).Scan(&legacy, &typed, &effects, &jobs); err != nil {
		t.Fatal(err)
	}
	if legacy != 1 || typed != 0 || effects != 0 || jobs != 0 {
		t.Fatalf("preexisting legacy/typed/effects/jobs = %d/%d/%d/%d, want 1/0/0/0", legacy, typed, effects, jobs)
	}
}

func requestAtomicLegacyMutation(t *testing.T, ctx context.Context, legacy *contactapp.LegacyTagLiveMutationService, effects *tag.Service, command tag.QueueCommand) (tag.Acceptance, int64) {
	t.Helper()
	legacyOperation := contactapp.LegacyTagLiveMutationMark
	if command.Operation == tag.OperationUnmark {
		legacyOperation = contactapp.LegacyTagLiveMutationUnmark
	}
	var effect tag.Acceptance
	var receiptID int64
	_, err := legacy.RequestWithCommitHook(ctx, contactapp.LegacyTagLiveMutationCommand{
		Actor: command.Actor, IdempotencyKey: command.IdempotencyKey, TraceID: "wc01-acceptance", Operation: legacyOperation,
		Payload: json.RawMessage(`{"source":"wc01-acceptance"}`),
	}, func(txCtx context.Context, acceptance contactapp.LegacyTagLiveMutationAcceptance, _ bool) error {
		receiptID = acceptance.ReceiptID
		command.LegacyReceiptID = acceptance.ReceiptID
		var queueErr error
		effect, queueErr = effects.QueueInTransaction(txCtx, command)
		return queueErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return effect, receiptID
}

func requestAtomicLegacySync(t *testing.T, ctx context.Context, legacy *contactapp.LegacyTagSyncService, effects *tag.Service, command tag.QueueCommand) (tag.Acceptance, int64) {
	t.Helper()
	kind := contactapp.LegacyTagSyncManual
	if command.SyncTrigger == tag.SyncTriggerDue {
		kind = contactapp.LegacyTagSyncDue
	}
	var effect tag.Acceptance
	var receiptID int64
	_, err := legacy.RequestWithCommitHook(ctx, contactapp.LegacyTagSyncCommand{
		Actor: command.Actor, IdempotencyKey: command.IdempotencyKey, TraceID: "wc01-acceptance", Kind: kind,
	}, func(txCtx context.Context, acceptance contactapp.LegacyTagSyncAcceptance, _ bool) error {
		receiptID = acceptance.ReceiptID
		command.LegacyReceiptID = acceptance.ReceiptID
		var queueErr error
		effect, queueErr = effects.QueueInTransaction(txCtx, command)
		return queueErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return effect, receiptID
}

func assertTagJobCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, tag.JobKind).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("tag River jobs = %d, want %d", got, want)
	}
}

func testDigest(value string) eerport.Digest {
	sum := sha256.Sum256([]byte(value))
	return eerport.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
