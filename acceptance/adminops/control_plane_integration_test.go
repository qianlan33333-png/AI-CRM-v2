package adminops_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	adminopsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestControlPlanePGReceiptSecretBoundaryAndJobIsolation(t *testing.T) {
	dsn := os.Getenv("ADMINOPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ADMINOPS_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service := adminopsapp.NewService(platformstore.NewUnitOfWork(pool), adminopsstore.NewRepository())
	unique := time.Now().UTC().Format("20060102150405.000000000")
	command := adminopsapp.CredentialCommand{Kind: adminopsport.CredentialAPIClient, ClientID: "test-" + unique, DisplayName: "test client", Metadata: map[string]any{"token_ttl_minutes": 30}, Actor: "admin:41", RequestID: "credential-" + unique}
	created, err := service.CreateCredential(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "pending_activation" || created.SecretRef == "" || created.SecretMask == "" {
		t.Fatalf("unsafe credential response: %#v", created)
	}
	replayed, err := service.CreateCredential(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != created.ID || replayed.SecretRef != created.SecretRef {
		t.Fatalf("receipt replay mismatch: %#v != %#v", replayed, created)
	}
	_, err = service.CreateCredential(ctx, adminopsapp.CredentialCommand{Kind: adminopsport.CredentialAPIClient, ClientID: "reject-" + unique, DisplayName: "reject", Metadata: map[string]any{"webhook_url": "https://secret.invalid"}, Actor: "admin:41", RequestID: "reject-" + unique})
	if !errors.Is(err, adminopsapp.ErrSecretMaterial) {
		t.Fatalf("raw secret handling error = %v", err)
	}
	release, err := service.CreateRelease(ctx, adminopsapp.ReleaseCommand{Changes: map[string]any{"outbound.rate_per_second": 1}, Actor: "admin:41", RequestID: "release-" + unique})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := service.ValidateRelease(ctx, release.ID, "admin:41", "release-validate-"+unique)
	if err != nil || validated.State != "validated" {
		t.Fatalf("validate = %#v, %v", validated, err)
	}
	published, err := service.PublishRelease(ctx, release.ID, release.Checksum, "admin:41", "release-publish-"+unique)
	if err != nil || published.State != "published" {
		t.Fatalf("publish = %#v, %v", published, err)
	}
	rolledBack, err := service.RollbackRelease(ctx, release.ID, "admin:41", "release-rollback-"+unique)
	if err != nil || rolledBack.State != "rolled_back" {
		t.Fatalf("rollback = %#v, %v", rolledBack, err)
	}
	job, err := service.EnqueueJob(ctx, adminopsapp.JobCommand{Kind: "feishu_webhook_validate", TargetRef: "secret://test/notification/" + unique, Actor: "admin:41", RequestID: "job-" + unique, Summary: map[string]any{"secret_ref": "secret://test/notification/" + unique}})
	if err != nil || job.State != "queued" {
		t.Fatalf("enqueue = %#v, %v", job, err)
	}
	unknown, err := service.MarkOutcomeUnknown(ctx, job.Key, "provider_result_not_observed", job.Version)
	if err != nil || unknown.State != "outcome_unknown" {
		t.Fatalf("unknown = %#v, %v", unknown, err)
	}
	if _, err := service.CancelJob(ctx, job.Key, unknown.Version, "admin:41", "cancel-"+unique); !errors.Is(err, adminopsapp.ErrInvalidTransition) {
		t.Fatalf("unknown cancellation error = %v", err)
	}
}
