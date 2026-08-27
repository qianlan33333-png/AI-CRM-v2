package media_acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	externaleffectsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type media83UnknownAdapter struct{}

func (media83UnknownAdapter) Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{}, errors.New("media83 deterministic unknown transport")
}

func TestP4MediaContentDelivery00083BusinessClosure(t *testing.T) {
	pool, ctx := openContentDelivery00083Pool(t)
	uow := platformstore.NewUnitOfWork(pool)
	contentRepository := mediastore.NewContentDeliveryRepository()
	content := mediaapp.NewContentDeliveryService(uow, contentRepository)
	attachments := mediaapp.NewAttachmentService(uow, mediastore.NewAttachmentRepository(), eventstore.NewAppender())

	firstAttachment := uploadContentDeliveryAttachment(t, ctx, attachments, 8301, "media83-attachment-key-0001", "first.pdf")
	secondAttachment := uploadContentDeliveryAttachment(t, ctx, attachments, 8301, "media83-attachment-key-0002", "second.pdf")
	if !firstAttachment.Enabled || !secondAttachment.Enabled {
		t.Fatalf("attachments are not enabled: first=%+v second=%+v", firstAttachment, secondAttachment)
	}
	create := mediaport.ContentPackageCommand{
		Name: "media83 package", ContentText: "local content", Enabled: true, Actor: 8301,
		IdempotencyKey: "media83-package-create-key-0001",
		Refs:           []mediaport.ContentRef{{Kind: "attachment", ID: firstAttachment.ID}},
	}
	created, err := content.Create(ctx, create)
	if err != nil || created.ID < 1 || created.Version != 1 || len(created.Refs) != 1 || created.Refs[0].ID != firstAttachment.ID {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	replayed, err := content.Create(ctx, create)
	if err != nil || replayed.ID != created.ID || replayed.Version != created.Version || len(replayed.Refs) != 1 || replayed.Refs[0] != created.Refs[0] {
		t.Fatalf("create replay=%+v err=%v", replayed, err)
	}
	changedCreate := create
	changedCreate.Name = "forged replay"
	if _, err = content.Create(ctx, changedCreate); !errors.Is(err, mediaapp.ErrContentDeliveryConflict) {
		t.Fatalf("create payload conflict err=%v", err)
	}

	update := mediaport.ContentPackageUpdateCommand{ID: created.ID, ExpectedVersion: 1, ContentPackageCommand: mediaport.ContentPackageCommand{
		Name: "media83 package v2", ContentText: "updated local content", Enabled: true, Actor: 8301,
		IdempotencyKey: "media83-package-update-key-0001",
		Refs:           []mediaport.ContentRef{{Kind: "attachment", ID: secondAttachment.ID}},
	}}
	updated, err := content.Update(ctx, update)
	if err != nil || updated.Version != 2 || len(updated.Refs) != 1 || updated.Refs[0].ID != secondAttachment.ID {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	replayedUpdate, err := content.Update(ctx, update)
	if err != nil || replayedUpdate.Version != updated.Version || replayedUpdate.Refs[0] != updated.Refs[0] {
		t.Fatalf("update replay=%+v err=%v", replayedUpdate, err)
	}
	var storedRef int64
	if err = pool.QueryRow(ctx, "SELECT attachment_id FROM media_content_package_refs WHERE package_id=$1 AND position=1", created.ID).Scan(&storedRef); err != nil || storedRef != secondAttachment.ID {
		t.Fatalf("stored ref=%d err=%v", storedRef, err)
	}
	changedUpdate := update
	changedUpdate.ContentText = "forged update replay"
	if _, err = content.Update(ctx, changedUpdate); !errors.Is(err, mediaapp.ErrContentDeliveryConflict) {
		t.Fatalf("update payload conflict err=%v", err)
	}

	uploadContent := append([]byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nstream\n"), bytes.Repeat([]byte("x"), (1<<20)+64)...)
	uploadContent = append(uploadContent, []byte("\nendstream\nendobj\n%%EOF\n")...)
	uploadDigest := sha256.Sum256(uploadContent)
	initiate := mediaport.AttachmentUploadInitiateCommand{FileName: "multipart.pdf", Name: "multipart", SHA256: "sha256:" + hex.EncodeToString(uploadDigest[:]), Size: int64(len(uploadContent)), Actor: 8301, Enabled: true, IdempotencyKey: "media83-upload-init-key-0001"}
	uploadID, err := content.InitiatePDF(ctx, initiate)
	if err != nil || uploadID < 1 {
		t.Fatalf("initiate id=%d err=%v", uploadID, err)
	}
	replayedUploadID, err := content.InitiatePDF(ctx, initiate)
	if err != nil || replayedUploadID != uploadID {
		t.Fatalf("initiate replay id=%d err=%v", replayedUploadID, err)
	}
	changedInitiate := initiate
	changedInitiate.Name = "forged multipart"
	if _, err = content.InitiatePDF(ctx, changedInitiate); !errors.Is(err, mediaapp.ErrContentDeliveryConflict) {
		t.Fatalf("initiate payload conflict err=%v", err)
	}
	firstPart, secondPart := uploadContent[:1<<20], uploadContent[1<<20:]
	if len(secondPart) == 0 {
		t.Fatal("multipart fixture did not cross the 1MiB boundary")
	}
	firstDigest := sha256.Sum256(firstPart)
	secondDigest := sha256.Sum256(secondPart)
	part := mediaport.AttachmentUploadPartCommand{UploadID: uploadID, PartNumber: 1, SHA256: "sha256:" + hex.EncodeToString(firstDigest[:]), Content: firstPart, Actor: 8301, IdempotencyKey: "media83-upload-part-key-0001"}
	tamperedPart := part
	tamperedPart.SHA256 = initiate.SHA256
	tamperedPart.IdempotencyKey = "media83-upload-part-tampered-key-0001"
	if err = content.PutPDFPart(ctx, tamperedPart); !errors.Is(err, mediaapp.ErrContentDeliveryInvalid) {
		t.Fatalf("tampered part digest err=%v", err)
	}
	if err = content.PutPDFPart(ctx, part); err != nil {
		t.Fatal(err)
	}
	if err = content.PutPDFPart(ctx, part); err != nil {
		t.Fatalf("part replay err=%v", err)
	}
	differentContent := append([]byte(nil), firstPart...)
	differentContent[len(differentContent)-2] = 'X'
	differentDigest := sha256.Sum256(differentContent)
	conflictingPart := part
	conflictingPart.Content = differentContent
	conflictingPart.SHA256 = "sha256:" + hex.EncodeToString(differentDigest[:])
	conflictingPart.IdempotencyKey = "media83-upload-part-key-0002"
	if err = content.PutPDFPart(ctx, conflictingPart); !errors.Is(err, mediaapp.ErrContentDeliveryConflict) {
		t.Fatalf("part content conflict err=%v", err)
	}
	if err = content.PutPDFPart(ctx, mediaport.AttachmentUploadPartCommand{UploadID: uploadID, PartNumber: 2, SHA256: "sha256:" + hex.EncodeToString(secondDigest[:]), Content: secondPart, Actor: 8301, IdempotencyKey: "media83-upload-part-key-0003"}); err != nil {
		t.Fatalf("second part err=%v", err)
	}
	incomplete := initiate
	incomplete.FileName = "multipart-incomplete.pdf"
	incomplete.Name = "multipart incomplete"
	incomplete.IdempotencyKey = "media83-upload-init-key-0002"
	incompleteUploadID, err := content.InitiatePDF(ctx, incomplete)
	if err != nil || incompleteUploadID < 1 {
		t.Fatalf("incomplete initiate id=%d err=%v", incompleteUploadID, err)
	}
	if err = content.PutPDFPart(ctx, mediaport.AttachmentUploadPartCommand{UploadID: incompleteUploadID, PartNumber: 2, SHA256: "sha256:" + hex.EncodeToString(secondDigest[:]), Content: secondPart, Actor: 8301, IdempotencyKey: "media83-upload-part-key-0004"}); err != nil {
		t.Fatalf("out-of-order part err=%v", err)
	}
	if _, err = content.CompletePDF(ctx, mediaport.AttachmentUploadCompleteCommand{UploadID: incompleteUploadID, Actor: 8301, IdempotencyKey: "media83-upload-complete-key-0002"}); !errors.Is(err, mediaapp.ErrContentDeliveryUnavailable) {
		t.Fatalf("out-of-order completion err=%v", err)
	}
	var incompleteAttachments int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM media_attachments WHERE file_name='multipart-incomplete.pdf'").Scan(&incompleteAttachments); err != nil || incompleteAttachments != 0 {
		t.Fatalf("out-of-order completion wrote attachment count=%d err=%v", incompleteAttachments, err)
	}
	complete := mediaport.AttachmentUploadCompleteCommand{UploadID: uploadID, Actor: 8301, IdempotencyKey: "media83-upload-complete-key-0001"}
	attachmentID, err := content.CompletePDF(ctx, complete)
	if err != nil || attachmentID < 1 {
		t.Fatalf("complete id=%d err=%v", attachmentID, err)
	}
	replayedAttachmentID, err := content.CompletePDF(ctx, complete)
	if err != nil || replayedAttachmentID != attachmentID {
		t.Fatalf("complete replay id=%d err=%v", replayedAttachmentID, err)
	}
	downloaded, err := attachments.Download(ctx, attachmentID)
	if err != nil || downloaded.Attachment.ID != attachmentID || !bytes.Equal(downloaded.Content, uploadContent) {
		t.Fatalf("multipart download=%+v bytes=%d err=%v", downloaded.Attachment, len(downloaded.Content), err)
	}
	downloadDigest := sha256.Sum256(downloaded.Content)
	if downloadDigest != uploadDigest {
		t.Fatalf("multipart download checksum=%x want=%x", downloadDigest, uploadDigest)
	}
	var attachmentChecksum, blobChecksum []byte
	if err = pool.QueryRow(ctx, "SELECT attachment.checksum, blob.checksum FROM media_attachments AS attachment JOIN media_attachment_blobs AS blob ON blob.attachment_id=attachment.id WHERE attachment.id=$1", attachmentID).Scan(&attachmentChecksum, &blobChecksum); err != nil || !bytes.Equal(attachmentChecksum, uploadDigest[:]) || !bytes.Equal(blobChecksum, uploadDigest[:]) {
		t.Fatalf("multipart persisted checksums attachment=%x blob=%x err=%v", attachmentChecksum, blobChecksum, err)
	}

	campaignCode, planID, firstInvite, secondInvite := seedContentDeliveryBindingPrerequisites(t, ctx, pool)
	bind := mediaport.DeliveryBindingCommand{CampaignCode: campaignCode, PlanID: planID, PackageID: created.ID, GroupInviteID: firstInvite, Actor: 8301, IdempotencyKey: "media83-binding-create-key-0001"}
	binding, err := content.Bind(ctx, bind)
	if err != nil || binding.ID < 1 || binding.Version != 1 {
		t.Fatalf("bind=%+v err=%v", binding, err)
	}
	replayedBinding, err := content.Bind(ctx, bind)
	if err != nil || replayedBinding.ID != binding.ID || replayedBinding.Version != binding.Version {
		t.Fatalf("bind replay=%+v err=%v", replayedBinding, err)
	}
	changedBind := bind
	changedBind.GroupInviteID = secondInvite
	if _, err = content.Bind(ctx, changedBind); !errors.Is(err, mediaapp.ErrContentDeliveryConflict) {
		t.Fatalf("bind payload conflict err=%v", err)
	}
	updateBind := bind
	updateBind.ExpectedVersion = 1
	updateBind.GroupInviteID = secondInvite
	updateBind.IdempotencyKey = "media83-binding-update-key-0001"
	updatedBinding, err := content.Bind(ctx, updateBind)
	if err != nil || updatedBinding.Version != 2 || updatedBinding.GroupInviteID != secondInvite {
		t.Fatalf("bind update=%+v err=%v", updatedBinding, err)
	}
	replayedUpdatedBinding, err := content.Bind(ctx, updateBind)
	if err != nil || replayedUpdatedBinding.Version != updatedBinding.Version {
		t.Fatalf("bind update replay=%+v err=%v", replayedUpdatedBinding, err)
	}

	runtimeRepository := externaleffectsstore.NewRepository(pool, uow)
	runtime, err := eer.NewService(runtimeRepository)
	if err != nil {
		t.Fatal(err)
	}
	publishedRepository := mediastore.NewPublishedOutboundRepository()
	published := mediaapp.NewPublishedOutboundService(uow, publishedRepository, mediaapp.NewOutboundMediaService(runtime), publishedRepository)
	effect, replay, err := published.AcceptPublishedContentPackageForOutbound(ctx, created.ID, "external_contact_7", "media83-outbound-accept-key-0001")
	if err != nil || replay || effect.ID == "" {
		t.Fatalf("outbound effect=%+v replay=%v err=%v", effect, replay, err)
	}
	replayedEffect, replay, err := published.AcceptPublishedContentPackageForOutbound(ctx, created.ID, "external_contact_7", "media83-outbound-accept-key-0001")
	if err != nil || !replay || replayedEffect.ID != effect.ID {
		t.Fatalf("outbound replay=%+v replay=%v err=%v", replayedEffect, replay, err)
	}
	assertContentDeliveryAcceptanceSnapshot(t, ctx, pool, created.ID, effect.ID, secondAttachment.ID)
	queued, _, err := runtime.Queue(ctx, eer.QueueCommand{EffectID: effect.ID, ReceiptKeyDigest: eer.Digest(contentDeliveryDigest("media83-queue")), Job: eer.RiverJobLink{JobID: 83001, Generation: 1, Queue: "external_effects", ArgsDigest: eer.Digest(contentDeliveryDigest("media83-args")), ScheduledAt: time.Now().UTC()}})
	if err != nil || queued.State != eer.StateQueued {
		t.Fatalf("queue=%+v err=%v", queued, err)
	}
	lease, _, err := runtime.Claim(ctx, eer.ClaimCommand{EffectID: effect.ID, WorkerDigest: eer.Digest(contentDeliveryDigest("media83-worker"))})
	if err != nil {
		t.Fatal(err)
	}
	unknown, _, runErr := runtime.RunAttempt(ctx, lease, media83UnknownAdapter{})
	if !errors.Is(runErr, eer.ErrAdapterFailure) || unknown.State != eer.StateOutcomeUnknown {
		t.Fatalf("unknown=%+v err=%v", unknown, runErr)
	}
	reconcileService := mediaapp.NewOutboundMediaReconcileService(uow, contentRepository, runtime)
	reconcileCommand := mediaapp.OutboundMediaReconcileCommand{ContentPackageID: created.ID, TargetRef: "external_contact_7", Generation: lease.Generation, Fence: lease.Fence, LeaseExpiresAt: lease.ExpiresAt, EvidenceDigest: contentDeliveryDigest("media83-verified-evidence"), IdempotencyKey: "media83-reconcile-key-0001", ProviderAccepted: true}
	reconciled, err := reconcileService.Reconcile(ctx, reconcileCommand)
	if err != nil || reconciled.State != string(eer.StateReconciled) || reconciled.Replay || !reconciled.ProviderAccepted || reconciled.DeliveryProven {
		t.Fatalf("reconciled=%+v err=%v", reconciled, err)
	}
	replayedReconcile, err := reconcileService.Reconcile(ctx, reconcileCommand)
	if err != nil || !replayedReconcile.Replay {
		t.Fatalf("reconcile replay=%+v err=%v", replayedReconcile, err)
	}
	conflictingReconcile := reconcileCommand
	conflictingReconcile.IdempotencyKey = "media83-reconcile-key-0002"
	if _, err = reconcileService.Reconcile(ctx, conflictingReconcile); !errors.Is(err, mediaapp.ErrOutboundMediaReconcileConflict) {
		t.Fatalf("reconcile key conflict err=%v", err)
	}
	var storedReconcileKeyDigest string
	if err = pool.QueryRow(ctx, "SELECT idempotency_key_digest FROM outbound_media_reconciliation_receipts WHERE effect_id=$1", strings.TrimPrefix(effect.ID, "eer_")).Scan(&storedReconcileKeyDigest); err != nil || storedReconcileKeyDigest != contentDeliveryDigest("outbound-media-manual-reconcile-key", reconcileCommand.IdempotencyKey) {
		t.Fatalf("reconcile key digest=%s err=%v", storedReconcileKeyDigest, err)
	}

	mutateAfterAccept := mediaport.ContentPackageUpdateCommand{ID: created.ID, ExpectedVersion: updated.Version, ContentPackageCommand: mediaport.ContentPackageCommand{
		Name: updated.Name, ContentText: "changed after acceptance", Enabled: true, Actor: 8301,
		IdempotencyKey: "media83-package-update-key-0002", Refs: updated.Refs,
	}}
	if _, err = content.Update(ctx, mutateAfterAccept); err != nil {
		t.Fatal(err)
	}
	var effectsBefore int64
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM external_effects WHERE owner='outbound' AND kind='outbound_media'").Scan(&effectsBefore); err != nil {
		t.Fatal(err)
	}
	if _, _, err = published.AcceptPublishedContentPackageForOutbound(ctx, created.ID, "external_contact_7", "media83-outbound-accept-key-0002"); err == nil {
		t.Fatal("changed snapshot was accepted over immutable target binding")
	}
	var effectsAfter int64
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM external_effects WHERE owner='outbound' AND kind='outbound_media'").Scan(&effectsAfter); err != nil || effectsAfter != effectsBefore {
		t.Fatalf("orphan EER count before=%d after=%d err=%v", effectsBefore, effectsAfter, err)
	}

	var receiptCount int64
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM media_content_package_mutation_receipts WHERE actor_id=$1", int64(8301)).Scan(&receiptCount); err != nil || receiptCount != 8 {
		t.Fatalf("receipt count=%d err=%v", receiptCount, err)
	}
}

func openContentDelivery00083Pool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, "aicrm_test_media_delivery_83"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func contentDeliveryDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func uploadContentDeliveryAttachment(t *testing.T, ctx context.Context, service *mediaapp.AttachmentService, actor int64, key, fileName string) mediaport.Attachment {
	t.Helper()
	value, err := service.Upload(ctx, mediaport.AttachmentUploadCommand{Actor: actor, IdempotencyKey: key, FileName: fileName, DeclaredType: "application/pdf", Content: []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n"), Name: fileName})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func seedContentDeliveryBindingPrerequisites(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string, int64, int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var firstInvite, secondInvite int64
	if err = tx.QueryRow(ctx, "INSERT INTO media_group_invites (name,title,join_url,enabled,created_by,updated_by,created_at,updated_at) VALUES ('media83-invite-1','Media83 invite 1','https://work.weixin.qq.com/gm/media83-1',true,1,1,now(),now()) RETURNING id").Scan(&firstInvite); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, "INSERT INTO media_group_invites (name,title,join_url,enabled,created_by,updated_by,created_at,updated_at) VALUES ('media83-invite-2','Media83 invite 2','https://work.weixin.qq.com/gm/media83-2',true,1,1,now(),now()) RETURNING id").Scan(&secondInvite); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return "media83-campaign", "ctp_" + strings.Repeat("8", 64), firstInvite, secondInvite
}

func assertContentDeliveryAcceptanceSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, packageID int64, effectID string, attachmentID int64) {
	t.Helper()
	var storedEffectID int64
	var mediaRefs []byte
	var state string
	if err := pool.QueryRow(ctx, "SELECT external_effect_id,media_refs,state FROM outbound_media_acceptances WHERE content_package_id=$1", packageID).Scan(&storedEffectID, &mediaRefs, &state); err != nil {
		t.Fatal(err)
	}
	if "eer_"+strconv.FormatInt(storedEffectID, 10) != effectID || state != "accepted" || !strings.Contains(string(mediaRefs), strconv.FormatInt(attachmentID, 10)) {
		t.Fatalf("snapshot effect=%d refs=%s state=%s", storedEffectID, mediaRefs, state)
	}
	if _, err := pool.Exec(ctx, "UPDATE outbound_media_acceptances SET state='queued' WHERE content_package_id=$1", packageID); err == nil {
		t.Fatal("immutable acceptance snapshot was updated")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
			t.Fatalf("immutable snapshot err=%v", err)
		}
	}
}
