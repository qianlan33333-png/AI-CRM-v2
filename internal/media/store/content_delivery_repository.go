package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
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
func (r *ContentDeliveryRepository) Complete(context.Context, mediaport.AttachmentUploadCompleteCommand, time.Time) (int64, error) {
	return 0, mediaapp.ErrContentDeliveryUnavailable
}

var _ = pgtype.Int8{}
