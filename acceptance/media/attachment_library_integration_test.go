package media_acceptance

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/automationfixture"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/radarfixture"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
)

// This suite deliberately accepts only the dedicated, disposable attachment
// database. It does not reset or modify the shared aicrm_test migration ledger.
func TestAttachmentLibrary00062PostgreSQLMigrationLifecycleAndRace(t *testing.T) {
	pool, ctx := openAttachmentPool(t)
	repoRoot := attachmentRepoRoot(t)
	assertAttachmentWaterline(t, ctx, pool, 62)
	assertAttachmentFactsEmpty(t, ctx, pool)

	// The initially empty dedicated database proves 00062 can go down and back
	// up after the full migration sequence without relying on a shared ledger.
	runAttachmentGoose(t, ctx, repoRoot, "down-to", "61")
	assertAttachmentWaterline(t, ctx, pool, 61)
	assertAttachmentTableAbsent(t, ctx, pool)
	runAttachmentGoose(t, ctx, repoRoot, "up-to", "62")
	assertAttachmentWaterline(t, ctx, pool, 62)
	assertAttachmentTablePresent(t, ctx, pool)

	service := realAttachmentService(pool)
	actor := int64(9620)
	command := attachmentCommand(actor, unique("attachment-upload"), "private-guide")
	created, err := service.Upload(ctx, command)
	if err != nil || created.ID < 1 || created.MimeType != "application/pdf" || created.FileSize < 1 || !created.Enabled || created.Version != 1 {
		t.Fatalf("upload=%+v err=%v", created, err)
	}
	replayed, err := service.Upload(ctx, command)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("upload replay=%+v err=%v", replayed, err)
	}
	changed := command
	changed.Description = "changed payload"
	if _, err = service.Upload(ctx, changed); !errors.Is(err, mediaapp.ErrAttachmentConflict) {
		t.Fatalf("changed upload replay err=%v", err)
	}

	page, err := service.List(ctx, mediaport.AttachmentListQuery{Limit: 100, EnabledOnly: true})
	if err != nil || page.Total < 1 || len(page.Items) < 1 {
		t.Fatalf("list=%+v err=%v", page, err)
	}
	download, err := service.Download(ctx, created.ID)
	if err != nil || download.Attachment.ID != created.ID || !strings.HasPrefix(string(download.Content), "%PDF-") {
		t.Fatalf("download=%+v err=%v", download.Attachment, err)
	}
	updated, err := service.Update(ctx, mediaport.AttachmentUpdateCommand{
		AttachmentID: created.ID, ExpectedVersion: created.Version, Actor: actor + 1,
		IdempotencyKey: unique("attachment-update"), Name: "Private guide v2", Description: "local private PDF",
		Tags: []string{"private", "guide"}, Enabled: false,
	})
	if err != nil || updated.Version != created.Version+1 || updated.Enabled || updated.UpdatedBy != actor+1 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	if _, err = service.Update(ctx, mediaport.AttachmentUpdateCommand{
		AttachmentID: created.ID, ExpectedVersion: created.Version, Actor: actor + 1,
		IdempotencyKey: unique("attachment-stale"), Name: "stale", Tags: []string{}, Enabled: false,
	}); !errors.Is(err, mediaapp.ErrAttachmentConflict) {
		t.Fatalf("stale CAS err=%v", err)
	}
	assertAttachmentDurableFacts(t, ctx, pool, created.ID)
	assertAttachmentBlobInvariant(t, ctx, pool, created.ID)

	for _, fixture := range []struct {
		name  string
		seed  func(*testing.T, context.Context, *pgxpool.Pool, int64) int64
		match func(mediaapp.AttachmentDeleteReferences) bool
	}{
		{"automation", seedAttachmentAutomationReference, func(refs mediaapp.AttachmentDeleteReferences) bool { return len(refs.AutomationAgents) == 1 }},
		{"channel", seedAttachmentChannelReference, func(refs mediaapp.AttachmentDeleteReferences) bool { return len(refs.Channels) == 1 }},
		{"radar", seedAttachmentRadarReference, func(refs mediaapp.AttachmentDeleteReferences) bool { return len(refs.RadarLinks) == 1 }},
	} {
		fixture := fixture
		t.Run("delete fails closed for "+fixture.name, func(t *testing.T) {
			attachment, uploadErr := service.Upload(ctx, attachmentCommand(actor, unique("attachment-reference-"+fixture.name), fixture.name+"-reference"))
			if uploadErr != nil {
				t.Fatal(uploadErr)
			}
			ownerID := fixture.seed(t, ctx, pool, attachment.ID)
			blocked, deleteErr := service.Delete(ctx, mediaport.AttachmentDeleteCommand{AttachmentID: attachment.ID, Actor: actor, IdempotencyKey: unique("attachment-delete-" + fixture.name)})
			if !errors.Is(deleteErr, mediaapp.ErrAttachmentHasReferences) || !fixture.match(blocked.References) || !attachmentReferenceListsSorted(blocked.References) {
				t.Fatalf("owner=%d delete=%+v err=%v", ownerID, blocked, deleteErr)
			}
			var attachments, receipts, events int
			if err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM media_attachments WHERE id=$1),
  (SELECT count(*) FROM media_attachment_mutation_receipts WHERE operation='delete' AND business_key=$1::text),
  (SELECT count(*) FROM event_log WHERE event_type='media.attachment_deleted' AND payload->>'attachment_id'=$1::text)`, attachment.ID).Scan(&attachments, &receipts, &events); err != nil || attachments != 1 || receipts != 0 || events != 0 {
				t.Fatalf("blocked facts=%d/%d/%d err=%v", attachments, receipts, events, err)
			}
		})
	}

	// A writer beginning after the attachment delete lock waits at FOR KEY
	// SHARE and, after the delete commits, rejects the now-missing attachment.
	raceAttachment, err := service.Upload(ctx, attachmentCommand(actor, unique("attachment-race-upload"), "race"))
	if err != nil {
		t.Fatal(err)
	}
	locked, release := make(chan struct{}, 1), make(chan struct{})
	observer := &attachmentDeleteLockObserver{AttachmentRepository: mediastore.NewAttachmentRepository(), locked: locked, release: release}
	deleteResults := make(chan error, 1)
	go func() {
		_, deleteErr := realAttachmentServiceWithStore(pool, observer).Delete(ctx, mediaport.AttachmentDeleteCommand{AttachmentID: raceAttachment.ID, Actor: actor, IdempotencyKey: unique("attachment-race-delete")})
		deleteResults <- deleteErr
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("attachment delete did not acquire row lock")
	}
	writerResults := make(chan error, 1)
	go func() {
		service, serviceErr := radarapp.NewServiceWithMediaReferences(platformstore.NewUnitOfWork(pool), radarstore.NewPostgresRepository(), mediastore.NewUploadRepository(), mediastore.NewAttachmentRepository(), eventstore.NewAppender())
		if serviceErr != nil {
			writerResults <- serviceErr
			return
		}
		_, writerErr := service.Create(ctx, radarport.CreateCommand{ExpectedVersion: 0, Name: unique("attachment-radar-race"), Title: "Attachment race", DestinationURL: "https://example.com/attachment-race", AttachmentID: &raceAttachment.ID, ActorID: actor, IdempotencyKey: unique("attachment-radar-writer")})
		writerResults <- writerErr
	}()
	select {
	case writerErr := <-writerResults:
		t.Fatalf("radar writer completed before attachment delete release: %v", writerErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case deleteErr := <-deleteResults:
		if deleteErr != nil {
			t.Fatalf("attachment delete=%v", deleteErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("attachment delete did not finish")
	}
	select {
	case writerErr := <-writerResults:
		if !errors.Is(writerErr, radarport.ErrInvalidArgument) {
			t.Fatalf("radar writer=%v", writerErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("radar attachment writer did not finish")
	}
	var attachmentRows, radarRows int
	if err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM media_attachments WHERE id=$1),
  (SELECT count(*) FROM radar_links WHERE attachment_id=$1)`, raceAttachment.ID).Scan(&attachmentRows, &radarRows); err != nil || attachmentRows != 0 || radarRows != 0 {
		t.Fatalf("race facts=%d/%d err=%v", attachmentRows, radarRows, err)
	}

	// Once local facts exist, the migration refuses to drop them. This is the
	// final failure-closed half of the same PG16 up/down/up exercise.
	err = attachmentGoose(ctx, repoRoot, *databaseURL, "down-to", "61")
	if err == nil || !strings.Contains(err.Error(), "55000") {
		t.Fatalf("rollback with facts err=%v, want SQLSTATE 55000", err)
	}
	assertAttachmentWaterline(t, ctx, pool, 62)
}

func realAttachmentService(pool *pgxpool.Pool) *mediaapp.AttachmentService {
	return realAttachmentServiceWithStore(pool, mediastore.NewAttachmentRepository())
}

func realAttachmentServiceWithStore(pool *pgxpool.Pool, store mediaapp.AttachmentStore) *mediaapp.AttachmentService {
	return mediaapp.NewAttachmentServiceWithReferences(platformstore.NewUnitOfWork(pool), store, automationstore.NewAgentRepository(), contactstore.NewChannelRepository(), radarstore.NewPostgresRepository(), eventstore.NewAppender())
}

func attachmentCommand(actor int64, key, name string) mediaport.AttachmentUploadCommand {
	return mediaport.AttachmentUploadCommand{
		Actor: actor, IdempotencyKey: key, FileName: "local-private.pdf", DeclaredType: "application/pdf",
		Content: []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n"), Name: name,
		Description: "private local attachment", Tags: []string{"private", "pdf"},
	}
}

func openAttachmentPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, acceptancefixtures.AttachmentLibraryDatabaseName); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	return pool, ctx
}

func attachmentRepoRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(directory, "migrations")); statErr == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root with migrations directory not found")
		}
		directory = parent
	}
}

func runAttachmentGoose(t *testing.T, ctx context.Context, repoRoot, operation, version string) {
	t.Helper()
	if err := attachmentGoose(ctx, repoRoot, *databaseURL, operation, version); err != nil {
		t.Fatal(err)
	}
}

func attachmentGoose(ctx context.Context, repoRoot, databaseURL, operation, version string) error {
	command := exec.CommandContext(ctx, "go", "tool", "-modfile=tools/go.mod", "goose", "-dir", "migrations", "postgres", databaseURL, operation, version)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("goose %s %s: %w: %s", operation, version, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func assertAttachmentWaterline(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&got); err != nil || got != want {
		t.Fatalf("migration waterline=%d want=%d err=%v", got, want, err)
	}
}

func assertAttachmentFactsEmpty(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var attachments, blobs, receipts int
	if err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM media_attachments),
  (SELECT count(*) FROM media_attachment_blobs),
  (SELECT count(*) FROM media_attachment_mutation_receipts)`).Scan(&attachments, &blobs, &receipts); err != nil || attachments != 0 || blobs != 0 || receipts != 0 {
		t.Fatalf("dedicated attachment database must start empty; facts=%d/%d/%d err=%v", attachments, blobs, receipts, err)
	}
}

func assertAttachmentTableAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var table *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.media_attachments')::text`).Scan(&table); err != nil || table != nil {
		t.Fatalf("media_attachments after down=%v err=%v", table, err)
	}
}

func assertAttachmentTablePresent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var table *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.media_attachments')::text`).Scan(&table); err != nil || table == nil {
		t.Fatalf("media_attachments after up=%v err=%v", table, err)
	}
}

func assertAttachmentDurableFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attachmentID int64) {
	t.Helper()
	var metadata, blobs, receipts, createdEvents, updatedEvents int
	if err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM media_attachments WHERE id=$1),
  (SELECT count(*) FROM media_attachment_blobs WHERE attachment_id=$1),
  (SELECT count(*) FROM media_attachment_mutation_receipts WHERE business_key IN ('upload', $1::text) AND state='completed'),
  (SELECT count(*) FROM event_log WHERE event_type='media.attachment_created' AND payload->>'attachment_id'=$1::text),
  (SELECT count(*) FROM event_log WHERE event_type='media.attachment_updated' AND payload->>'attachment_id'=$1::text)`, attachmentID).Scan(&metadata, &blobs, &receipts, &createdEvents, &updatedEvents); err != nil || metadata != 1 || blobs != 1 || receipts < 2 || createdEvents != 1 || updatedEvents != 1 {
		t.Fatalf("attachment durable facts=%d/%d/%d/%d/%d err=%v", metadata, blobs, receipts, createdEvents, updatedEvents, err)
	}
}

func assertAttachmentBlobInvariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attachmentID int64) {
	t.Helper()
	checksum := sha256.Sum256([]byte("orphan attachment"))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_attachments (
  name,file_name,mime_type,file_size,checksum,description,tags,enabled,version,created_by,updated_by,created_at,updated_at
) VALUES ('orphan attachment','orphan.pdf','application/pdf',1,$1,'','[]'::jsonb,true,1,1,1,now(),now())`, checksum[:]); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	assertAttachmentSQLState(t, tx.Commit(ctx), "23514")

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM media_attachment_blobs WHERE attachment_id=$1`, attachmentID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	assertAttachmentSQLState(t, tx.Commit(ctx), "23514")
	var blobs int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM media_attachment_blobs WHERE attachment_id=$1`, attachmentID).Scan(&blobs); err != nil || blobs != 1 {
		t.Fatalf("blob invariant persisted blobs=%d err=%v", blobs, err)
	}
}

func assertAttachmentSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != want {
		t.Fatalf("SQLSTATE error=%v want=%s", err, want)
	}
}

func seedAttachmentAutomationReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attachmentID int64) int64 {
	t.Helper()
	id, err := automationfixture.CreateAttachmentReference(ctx, pool, unique("attachment-agent"), unique("attachment-agent-code"), attachmentID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedAttachmentChannelReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attachmentID int64) int64 {
	t.Helper()
	id, err := contactfixture.CreateAttachmentReference(ctx, pool, unique("attachment-channel"), unique("attachment-channel-code"), attachmentID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedAttachmentRadarReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attachmentID int64) int64 {
	t.Helper()
	code := fmt.Sprintf("rd_%022d", attachmentID)
	id, err := radarfixture.CreateAttachmentReference(ctx, pool, code, unique("attachment-radar"), unique("attachment radar"), attachmentID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func attachmentReferenceListsSorted(references mediaapp.AttachmentDeleteReferences) bool {
	for _, ids := range [][]int64{references.AutomationAgents, references.Channels, references.RadarLinks} {
		for index, id := range ids {
			if id < 1 || index > 0 && ids[index-1] >= id {
				return false
			}
		}
	}
	return true
}

type attachmentDeleteLockObserver struct {
	*mediastore.AttachmentRepository
	locked  chan<- struct{}
	release <-chan struct{}
}

func (store *attachmentDeleteLockObserver) GetAttachmentForUpdate(ctx context.Context, attachmentID int64) (mediaport.Attachment, error) {
	attachment, err := store.AttachmentRepository.GetAttachmentForUpdate(ctx, attachmentID)
	if err != nil || attachment.ID < 1 {
		return attachment, err
	}
	select {
	case store.locked <- struct{}{}:
	case <-ctx.Done():
		return mediaport.Attachment{}, ctx.Err()
	}
	select {
	case <-store.release:
		return attachment, nil
	case <-ctx.Done():
		return mediaport.Attachment{}, ctx.Err()
	}
}
