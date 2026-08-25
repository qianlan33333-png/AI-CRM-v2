package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	"time"
)

type ContentDeliveryRepository struct{}

func NewContentDeliveryRepository() *ContentDeliveryRepository { return &ContentDeliveryRepository{} }

var _ mediaapp.ContentDeliveryStore = (*ContentDeliveryRepository)(nil)

func contentQueries(ctx context.Context) (*mediadb.Queries, error) { return queries(ctx) }
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
	for i, ref := range c.Refs {
		p := mediadb.InsertMediaContentPackageImageRefParams{PackageID: row.ID, Position: int32(i + 1), RefID: pgtype.Int8{Int64: ref.ID, Valid: true}}
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
			return mediaport.ContentPackage{}, errors.New("kind")
		}
		if e != nil {
			return mediaport.ContentPackage{}, e
		}
	}
	return mediaport.ContentPackage{ID: row.ID, Name: row.Name, ContentText: row.ContentText, Enabled: row.Enabled, Version: row.Version, Refs: c.Refs}, nil
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
	return mediaport.ContentPackage{ID: row.ID, Name: row.Name, ContentText: row.ContentText, Enabled: row.Enabled, Version: row.Version, Refs: c.Refs}, e
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
	e = q.PutMediaAttachmentUploadPart(ctx, mediadb.PutMediaAttachmentUploadPartParams{UploadID: c.UploadID, PartNumber: c.PartNumber, Digest: d[:], Content: c.Content, Now: stamp(n)})
	return e == nil, e
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
