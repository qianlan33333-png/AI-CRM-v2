package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	"strconv"
	"strings"
	"time"
)

type ContentDeliveryRepository struct{}

type OutboundMediaEffectBinding struct {
	ID, ContentPackageID, EffectID int64
	TargetDigest, SnapshotDigest   string
	Replay                         bool
}

var ErrOutboundMediaEffectBindingConflict = errors.New("outbound media effect binding conflict")

func NewContentDeliveryRepository() *ContentDeliveryRepository { return &ContentDeliveryRepository{} }

var _ mediaapp.ContentDeliveryStore = (*ContentDeliveryRepository)(nil)
var _ mediaapp.OutboundMediaReconcileStore = (*ContentDeliveryRepository)(nil)

func contentQueries(ctx context.Context) (*mediadb.Queries, error) { return queries(ctx) }
func (r *ContentDeliveryRepository) ReserveMutation(ctx context.Context, reservation mediaapp.ContentDeliveryMutationReservation) (mediaapp.ContentDeliveryMutationReceipt, bool, error) {
	q, err := contentQueries(ctx)
	if err != nil {
		return mediaapp.ContentDeliveryMutationReceipt{}, false, err
	}
	row, err := q.ReserveMediaContentPackageMutation(ctx, mediadb.ReserveMediaContentPackageMutationParams{Operation: reservation.Operation, ActorID: reservation.Actor, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: stamp(reservation.CreatedAt)})
	if err == nil {
		return contentMutationReceipt(row.ID, row.Operation, row.ActorID, row.KeyDigest, row.PayloadDigest, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ContentDeliveryMutationReceipt{}, false, err
	}
	existing, err := q.GetMediaContentPackageMutation(ctx, mediadb.GetMediaContentPackageMutationParams{Operation: reservation.Operation, ActorID: reservation.Actor, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return mediaapp.ContentDeliveryMutationReceipt{}, false, err
	}
	return contentMutationReceipt(existing.ID, existing.Operation, existing.ActorID, existing.KeyDigest, existing.PayloadDigest, existing.ResultSnapshot), false, nil
}
func (r *ContentDeliveryRepository) CompleteMutation(ctx context.Context, id int64, snapshot json.RawMessage) (mediaapp.ContentDeliveryMutationReceipt, error) {
	q, err := contentQueries(ctx)
	if err != nil || id < 1 || !json.Valid(snapshot) {
		if err != nil {
			return mediaapp.ContentDeliveryMutationReceipt{}, err
		}
		return mediaapp.ContentDeliveryMutationReceipt{}, mediaapp.ErrContentDeliveryUnavailable
	}
	row, err := q.CompleteMediaContentPackageMutation(ctx, mediadb.CompleteMediaContentPackageMutationParams{ID: id, ResultSnapshot: snapshot})
	if err != nil {
		return mediaapp.ContentDeliveryMutationReceipt{}, err
	}
	return contentMutationReceipt(row.ID, row.Operation, row.ActorID, row.KeyDigest, row.PayloadDigest, row.ResultSnapshot), nil
}
func contentMutationReceipt(id int64, operation string, actor int64, key, payload, snapshot []byte) mediaapp.ContentDeliveryMutationReceipt {
	var keyDigest, payloadDigest [32]byte
	copy(keyDigest[:], key)
	copy(payloadDigest[:], payload)
	return mediaapp.ContentDeliveryMutationReceipt{ID: id, Operation: operation, Actor: actor, KeyDigest: keyDigest, PayloadDigest: payloadDigest, ResultSnapshot: append(json.RawMessage(nil), snapshot...)}
}
func (r *ContentDeliveryRepository) Eligible(ctx context.Context, k string, id int64) (bool, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return false, e
	}
	return q.GetMediaContentReferenceEligibility(ctx, mediadb.GetMediaContentReferenceEligibilityParams{RefKind: k, RefID: id})
}
func (r *ContentDeliveryRepository) Create(ctx context.Context, c mediaport.ContentPackageCommand, n time.Time) (mediaport.ContentPackage, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return mediaport.ContentPackage{}, e
	}
	row, e := q.CreateMediaContentPackage(ctx, mediadb.CreateMediaContentPackageParams{Name: c.Name, ContentText: c.ContentText, Enabled: c.Enabled, ActorID: c.Actor, Now: stamp(n)})
	if e != nil {
		return mediaport.ContentPackage{}, e
	}
	if e = insertMediaContentPackageRefs(ctx, q, row.ID, c.Refs); e != nil {
		return mediaport.ContentPackage{}, e
	}
	refs, e := readMediaContentPackageRefs(ctx, q, row.ID)
	if e != nil {
		return mediaport.ContentPackage{}, e
	}
	return mediaport.ContentPackage{ID: row.ID, Name: row.Name, ContentText: row.ContentText, Enabled: row.Enabled, Version: row.Version, Refs: refs}, nil
}
func insertMediaContentPackageRefs(ctx context.Context, q *mediadb.Queries, packageID int64, refs []mediaport.ContentRef) error {
	for i, ref := range refs {
		p := mediadb.InsertMediaContentPackageImageRefParams{PackageID: packageID, Position: int32(i + 1), RefID: pgtype.Int8{Int64: ref.ID, Valid: true}}
		var e error
		switch ref.Kind {
		case "image":
			e = q.InsertMediaContentPackageImageRef(ctx, p)
		case "attachment":
			e = q.InsertMediaContentPackageAttachmentRef(ctx, mediadb.InsertMediaContentPackageAttachmentRefParams(p))
		case "miniprogram":
			e = q.InsertMediaContentPackageMiniprogramRef(ctx, mediadb.InsertMediaContentPackageMiniprogramRefParams(p))
		case "group_invite":
			e = q.InsertMediaContentPackageGroupInviteRef(ctx, mediadb.InsertMediaContentPackageGroupInviteRefParams(p))
		default:
			return errors.New("kind")
		}
		if e != nil {
			return e
		}
	}
	return nil
}
func readMediaContentPackageRefs(ctx context.Context, q *mediadb.Queries, packageID int64) ([]mediaport.ContentRef, error) {
	rows, err := q.ListMediaContentPackageRefs(ctx, packageID)
	if err != nil {
		return nil, err
	}
	refs := make([]mediaport.ContentRef, 0, len(rows))
	for _, row := range rows {
		if !row.RefID.Valid || row.RefID.Int64 < 1 {
			return nil, mediaapp.ErrContentDeliveryUnavailable
		}
		refs = append(refs, mediaport.ContentRef{Kind: row.RefKind, ID: row.RefID.Int64})
	}
	return refs, nil
}
func (r *ContentDeliveryRepository) Update(ctx context.Context, c mediaport.ContentPackageUpdateCommand, n time.Time) (mediaport.ContentPackage, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return mediaport.ContentPackage{}, e
	}
	row, e := q.UpdateMediaContentPackage(ctx, mediadb.UpdateMediaContentPackageParams{Name: c.Name, ContentText: c.ContentText, Enabled: c.Enabled, ActorID: c.Actor, Now: stamp(n), PackageID: c.ID, ExpectedVersion: c.ExpectedVersion})
	if errors.Is(e, pgx.ErrNoRows) {
		return mediaport.ContentPackage{}, mediaapp.ErrContentDeliveryConflict
	}
	if e != nil {
		return mediaport.ContentPackage{}, e
	}
	if e = q.DeleteMediaContentPackageRefs(ctx, c.ID); e != nil {
		return mediaport.ContentPackage{}, e
	}
	if e = insertMediaContentPackageRefs(ctx, q, c.ID, c.Refs); e != nil {
		return mediaport.ContentPackage{}, e
	}
	refs, e := readMediaContentPackageRefs(ctx, q, c.ID)
	if e != nil {
		return mediaport.ContentPackage{}, e
	}
	return mediaport.ContentPackage{ID: row.ID, Name: row.Name, ContentText: row.ContentText, Enabled: row.Enabled, Version: row.Version, Refs: refs}, nil
}
func (r *ContentDeliveryRepository) Bind(ctx context.Context, c mediaport.DeliveryBindingCommand, n time.Time) (mediaport.DeliveryBinding, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return mediaport.DeliveryBinding{}, e
	}
	if c.ExpectedVersion == 0 {
		v, e := q.CreateMediaCampaignDeliveryBinding(ctx, mediadb.CreateMediaCampaignDeliveryBindingParams{CampaignCode: c.CampaignCode, PlanID: c.PlanID, PackageID: c.PackageID, GroupInviteID: c.GroupInviteID, ActorID: c.Actor, Now: stamp(n)})
		return binding(v), e
	}
	v, e := q.UpdateMediaCampaignDeliveryBinding(ctx, mediadb.UpdateMediaCampaignDeliveryBindingParams{PackageID: c.PackageID, GroupInviteID: c.GroupInviteID, ActorID: c.Actor, Now: stamp(n), CampaignCode: c.CampaignCode, PlanID: c.PlanID, ExpectedVersion: c.ExpectedVersion})
	return binding(v), e
}
func (r *ContentDeliveryRepository) GetBinding(ctx context.Context, campaignCode, planID string) (mediaport.DeliveryBinding, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return mediaport.DeliveryBinding{}, e
	}
	v, e := q.GetMediaCampaignDeliveryBinding(ctx, mediadb.GetMediaCampaignDeliveryBindingParams{CampaignCode: campaignCode, PlanID: planID})
	return binding(v), e
}
func (r *ContentDeliveryRepository) BindOutboundMediaEffect(ctx context.Context, contentPackageID int64, targetDigest, snapshotDigest string, effectID int64, now time.Time) (OutboundMediaEffectBinding, error) {
	q, err := contentQueries(ctx)
	if err != nil || contentPackageID < 1 || effectID < 1 {
		return OutboundMediaEffectBinding{}, err
	}
	row, err := q.InsertOutboundMediaEffectBinding(ctx, mediadb.InsertOutboundMediaEffectBindingParams{ContentPackageID: contentPackageID, TargetDigest: targetDigest, SnapshotDigest: snapshotDigest, EffectID: effectID, CreatedAt: stamp(now)})
	if err == nil {
		return OutboundMediaEffectBinding{ID: row.ID, ContentPackageID: row.ContentPackageID, EffectID: row.EffectID, TargetDigest: row.TargetDigest, SnapshotDigest: row.SnapshotDigest}, nil
	}
	existing, readErr := q.GetOutboundMediaEffectBinding(ctx, mediadb.GetOutboundMediaEffectBindingParams{ContentPackageID: contentPackageID, TargetDigest: targetDigest})
	if readErr != nil {
		return OutboundMediaEffectBinding{}, err
	}
	if existing.SnapshotDigest != snapshotDigest || existing.EffectID != effectID {
		return OutboundMediaEffectBinding{}, ErrOutboundMediaEffectBindingConflict
	}
	return OutboundMediaEffectBinding{ID: existing.ID, ContentPackageID: existing.ContentPackageID, EffectID: existing.EffectID, TargetDigest: existing.TargetDigest, SnapshotDigest: existing.SnapshotDigest, Replay: true}, nil
}
func (r *ContentDeliveryRepository) ReadOutboundMediaEffectDetail(ctx context.Context, contentPackageID int64, targetDigest string) (mediaapp.OutboundMediaEffectDetail, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return mediaapp.OutboundMediaEffectDetail{}, e
	}
	v, e := q.ReadOutboundMediaEffectDetail(ctx, mediadb.ReadOutboundMediaEffectDetailParams{ContentPackageID: contentPackageID, TargetDigest: targetDigest})
	if e != nil {
		return mediaapp.OutboundMediaEffectDetail{}, e
	}
	return outboundMediaEffectDetail(contentPackageID, v.EffectID, v.State, v.ProviderAccepted, v.DeliveryProven), nil
}

func outboundMediaEffectDetail(contentPackageID, effectID int64, state string, providerAccepted, deliveryProven bool) mediaapp.OutboundMediaEffectDetail {
	return mediaapp.OutboundMediaEffectDetail{
		ContentPackageID: contentPackageID,
		EffectID:         "eer_" + strconv.FormatInt(effectID, 10),
		State:            state,
		ProviderAccepted: providerAccepted,
		DeliveryProven:   deliveryProven,
	}
}

func (r *ContentDeliveryRepository) LockOutboundMediaEffectForReconcile(ctx context.Context, contentPackageID int64, targetDigest string) (mediaapp.OutboundMediaReconcileControl, error) {
	q, err := contentQueries(ctx)
	if err != nil {
		return mediaapp.OutboundMediaReconcileControl{}, err
	}
	row, err := q.LockOutboundMediaEffectForReconcile(ctx, mediadb.LockOutboundMediaEffectForReconcileParams{ContentPackageID: contentPackageID, TargetDigest: targetDigest})
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.OutboundMediaReconcileControl{}, mediaapp.ErrOutboundMediaReconcileConflict
	}
	if err != nil {
		return mediaapp.OutboundMediaReconcileControl{}, err
	}
	if row.EffectID < 1 {
		return mediaapp.OutboundMediaReconcileControl{}, mediaapp.ErrOutboundMediaReconcileConflict
	}
	control := mediaapp.OutboundMediaReconcileControl{EffectID: "eer_" + strconv.FormatInt(row.EffectID, 10), State: row.State, Generation: row.Generation, Fence: row.LeaseFence}
	if row.LeaseExpiresAt.Valid {
		control.LeaseExpiresAt = row.LeaseExpiresAt.Time
	}
	return control, nil
}

func (r *ContentDeliveryRepository) ReadOutboundMediaReconciliationReceipt(ctx context.Context, effectID string) (mediaapp.OutboundMediaReconciliationReceipt, bool, error) {
	id, err := parseOutboundMediaEffectID(effectID)
	if err != nil {
		return mediaapp.OutboundMediaReconciliationReceipt{}, false, mediaapp.ErrOutboundMediaReconcileConflict
	}
	q, err := contentQueries(ctx)
	if err != nil {
		return mediaapp.OutboundMediaReconciliationReceipt{}, false, err
	}
	row, err := q.GetOutboundMediaReconciliationReceipt(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.OutboundMediaReconciliationReceipt{}, false, nil
	}
	if err != nil {
		return mediaapp.OutboundMediaReconciliationReceipt{}, false, err
	}
	if !row.LeaseExpiresAt.Valid {
		return mediaapp.OutboundMediaReconciliationReceipt{}, false, mediaapp.ErrOutboundMediaReconcileConflict
	}
	return mediaapp.OutboundMediaReconciliationReceipt{EffectID: effectID, Generation: row.Generation, Fence: row.Fence, LeaseExpiresAt: row.LeaseExpiresAt.Time, EvidenceDigest: row.EvidenceDigest, IdempotencyKeyDigest: row.IdempotencyKeyDigest, ProviderAccepted: row.ProviderAccepted, DeliveryProven: row.DeliveryProven, EERReceiptDigest: row.EerReceiptDigest}, true, nil
}

func (r *ContentDeliveryRepository) RecordOutboundMediaReconciliationReceipt(ctx context.Context, receipt mediaapp.OutboundMediaReconciliationReceipt) error {
	id, err := parseOutboundMediaEffectID(receipt.EffectID)
	if err != nil || receipt.Generation < 1 || receipt.Fence < 1 || receipt.LeaseExpiresAt.IsZero() || !validOutboundMediaDigest(receipt.EvidenceDigest) || !validOutboundMediaDigest(receipt.IdempotencyKeyDigest) || !validOutboundMediaDigest(receipt.EERReceiptDigest) || (receipt.DeliveryProven && !receipt.ProviderAccepted) {
		return mediaapp.ErrOutboundMediaReconcileConflict
	}
	q, err := contentQueries(ctx)
	if err != nil {
		return err
	}
	err = q.InsertOutboundMediaReconciliationReceipt(ctx, mediadb.InsertOutboundMediaReconciliationReceiptParams{EffectID: id, Generation: receipt.Generation, Fence: receipt.Fence, LeaseExpiresAt: stamp(receipt.LeaseExpiresAt), EvidenceDigest: receipt.EvidenceDigest, IdempotencyKeyDigest: receipt.IdempotencyKeyDigest, ProviderAccepted: receipt.ProviderAccepted, DeliveryProven: receipt.DeliveryProven, EerReceiptDigest: receipt.EERReceiptDigest, CreatedAt: stamp(time.Now().UTC())})
	if err != nil {
		return err
	}
	return nil
}

func parseOutboundMediaEffectID(value string) (int64, error) {
	if !strings.HasPrefix(value, "eer_") {
		return 0, errors.New("effect id")
	}
	digits := strings.TrimPrefix(value, "eer_")
	if digits == "" || digits[0] == '0' {
		return 0, errors.New("effect id")
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return 0, errors.New("effect id")
		}
	}
	return strconv.ParseInt(digits, 10, 64)
}

func validOutboundMediaDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value[7:])
	return err == nil
}
func binding(v mediadb.MediaCampaignDeliveryBinding) mediaport.DeliveryBinding {
	return mediaport.DeliveryBinding{ID: v.ID, CampaignCode: v.CampaignCode, PlanID: v.PlanID, PackageID: v.PackageID, GroupInviteID: v.GroupInviteID, Version: v.Version}
}
func (r *ContentDeliveryRepository) Initiate(ctx context.Context, c mediaport.AttachmentUploadInitiateCommand, d [32]byte, n time.Time) (int64, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return 0, e
	}
	v, e := q.InitiateMediaAttachmentUpload(ctx, mediadb.InitiateMediaAttachmentUploadParams{FileName: c.FileName, Name: c.Name, Description: c.Description, Tags: []byte("[]"), Enabled: c.Enabled, ExpectedSize: int32(c.Size), ExpectedDigest: d[:], ActorID: c.Actor, Now: stamp(n)})
	return v.ID, e
}
func (r *ContentDeliveryRepository) PutPart(ctx context.Context, c mediaport.AttachmentUploadPartCommand, d [32]byte, n time.Time) (bool, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return false, e
	}
	rows, e := q.PutMediaAttachmentUploadPart(ctx, mediadb.PutMediaAttachmentUploadPartParams{UploadID: c.UploadID, PartNumber: c.PartNumber, Digest: d[:], Content: c.Content, Now: stamp(n)})
	if e != nil || rows == 1 {
		return rows == 1, e
	}
	existing, readErr := q.GetMediaAttachmentUploadPart(ctx, mediadb.GetMediaAttachmentUploadPartParams{UploadID: c.UploadID, PartNumber: c.PartNumber})
	if readErr != nil {
		return false, readErr
	}
	return bytes.Equal(existing.Digest, d[:]) && bytes.Equal(existing.Content, c.Content), nil
}
func (r *ContentDeliveryRepository) Complete(ctx context.Context, command mediaport.AttachmentUploadCompleteCommand, now time.Time) (int64, error) {
	q, err := contentQueries(ctx)
	if err != nil {
		return 0, err
	}
	upload, err := q.ReadMediaAttachmentUploadForCompletion(ctx, command.UploadID)
	if err != nil {
		return 0, err
	}
	if upload.CreatedBy != command.Actor {
		return 0, mediaapp.ErrContentDeliveryConflict
	}
	if upload.State == "completed" && upload.AttachmentID.Valid && upload.AttachmentID.Int64 > 0 {
		return upload.AttachmentID.Int64, nil
	}
	parts, err := q.ListMediaAttachmentUploadParts(ctx, command.UploadID)
	if err != nil || len(parts) == 0 {
		return 0, mediaapp.ErrContentDeliveryUnavailable
	}
	content := make([]byte, 0, upload.ExpectedSize)
	for index, part := range parts {
		if part.PartNumber != int32(index+1) || len(part.Digest) != sha256.Size {
			return 0, mediaapp.ErrContentDeliveryInvalid
		}
		digest := sha256.Sum256(part.Content)
		if !bytes.Equal(digest[:], part.Digest) {
			return 0, mediaapp.ErrContentDeliveryInvalid
		}
		content = append(content, part.Content...)
	}
	checksum := sha256.Sum256(content)
	if len(content) != int(upload.ExpectedSize) || !bytes.Equal(checksum[:], upload.ExpectedDigest) {
		return 0, mediaapp.ErrContentDeliveryInvalid
	}
	if _, err = domain.InspectAttachment(upload.FileName, "application/pdf", content); err != nil {
		return 0, mediaapp.ErrContentDeliveryInvalid
	}
	attachment, err := q.InsertMediaAttachment(ctx, mediadb.InsertMediaAttachmentParams{Name: upload.Name, FileName: upload.FileName, MimeType: "application/pdf", FileSize: int32(len(content)), Checksum: checksum[:], Description: upload.Description, Tags: upload.Tags, Enabled: upload.Enabled, CreatedBy: command.Actor, UpdatedBy: command.Actor, CreatedAt: stamp(now), UpdatedAt: stamp(now)})
	if err != nil {
		return 0, err
	}
	if err = q.InsertMediaAttachmentBlob(ctx, mediadb.InsertMediaAttachmentBlobParams{AttachmentID: attachment.ID, Content: content, Checksum: checksum[:], CreatedAt: stamp(now)}); err != nil {
		return 0, err
	}
	if _, err = q.CompleteMediaAttachmentUpload(ctx, mediadb.CompleteMediaAttachmentUploadParams{AttachmentID: pgtype.Int8{Int64: attachment.ID, Valid: true}, Now: stamp(now), UploadID: command.UploadID}); err != nil {
		return 0, err
	}
	return attachment.ID, nil
}

var _ = pgtype.Int8{}
