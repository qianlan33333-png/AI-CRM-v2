package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerport "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/profile"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

type profileWriterFunc func(context.Context, wecomport.ContactProfileWriteRequest) (eerport.AdapterResult, error)

func (f profileWriterFunc) WriteContactProfile(ctx context.Context, r wecomport.ContactProfileWriteRequest) (eerport.AdapterResult, error) {
	return f(ctx, r)
}

func TestB1WC02WeComContactProfileEffectPG16(t *testing.T) {
	dsn := os.Getenv("P4B1WC02_WECOM_PROFILE_EFFECT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("P4B1WC02_WECOM_PROFILE_EFFECT_TEST_DATABASE_URL is not configured")
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
	runtime, err := eer.NewService(eerstore.NewRepository(pool, uow))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := profile.NewRiverJobInserter(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := profile.NewService(uow, wecomstore.NewProfileEffectRepository(pool), runtime, jobs, "corp-wc02")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := service.Queue(ctx, profile.QueueCommand{LegacyReceiptID: 9001, Actor: 71, IdempotencyKey: "wc02-profile-queue-0001", StaffUserID: "staff-wc02", ExternalUserID: "external-wc02", Remark: "VIP", Description: "follow up"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Queue(ctx, profile.QueueCommand{LegacyReceiptID: 9001, Actor: 71, IdempotencyKey: "wc02-profile-queue-0001", StaffUserID: "staff-wc02", ExternalUserID: "external-wc02", Remark: "VIP", Description: "follow up"})
	if err != nil || replay != accepted {
		t.Fatalf("queue replay=%+v first=%+v err=%v", replay, accepted, err)
	}
	calls := 0
	writer := profileWriterFunc(func(_ context.Context, r wecomport.ContactProfileWriteRequest) (eerport.AdapterResult, error) {
		calls++
		if r.Remark != "VIP" || r.StaffUserID != "staff-wc02" {
			t.Fatalf("provider request=%+v", r)
		}
		return eerport.AdapterResult{Completion: eerport.CompletionOutcomeUnknown, ReceiptDigest: wc02Digest("unknown"), BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
	})
	executed, err := service.Execute(ctx, accepted.EffectID, wc02Digest("worker"), writer)
	if err != nil || executed.State != eerport.StateOutcomeUnknown || !executed.ProviderCallAttempted || !executed.RealExternalCallExecuted || calls != 1 {
		t.Fatalf("unknown=%+v calls=%d err=%v", executed, calls, err)
	}
	replayed, err := service.Execute(ctx, accepted.EffectID, wc02Digest("worker-replay"), writer)
	if err != nil || replayed.ProviderCallAttempted || calls != 1 {
		t.Fatalf("unknown replay=%+v calls=%d err=%v", replayed, calls, err)
	}
	record, err := wecomstore.NewProfileEffectRepository(pool).Get(ctx, accepted.EffectID)
	if err != nil || record.AttemptReceiptID == "" || !record.ProviderCallAttempted || !record.RealExternalCallExecuted {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	reconciled, err := service.Reconcile(ctx, profile.ReconcileCommand{EffectID: accepted.EffectID, Actor: 72, IdempotencyKey: "wc02-profile-reconcile-0001", Generation: record.Generation, Fence: record.Fence, LeaseExpiresAt: record.LeaseExpiresAt, EvidenceDigest: wc02Digest("evidence"), Resolution: profile.ResolutionProviderApplied})
	if err != nil || reconciled.State != eerport.StateReconciled {
		t.Fatalf("reconcile=%+v err=%v", reconciled, err)
	}
	_, err = pool.Exec(ctx, "DELETE FROM public.wecom_contact_profile_effects WHERE effect_id=$1", mustWC02ID(t, accepted.EffectID))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
		t.Fatalf("delete err=%v want SQLSTATE 55000", err)
	}
}
func wc02Digest(label string) eerport.Digest {
	sum := sha256.Sum256([]byte("wc02\x00" + label))
	return eerport.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
func mustWC02ID(t *testing.T, v string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(v, "eer_%d", &id); err != nil || id < 1 {
		t.Fatalf("effect id=%q err=%v", v, err)
	}
	return id
}
