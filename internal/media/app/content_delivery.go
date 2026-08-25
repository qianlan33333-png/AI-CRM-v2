package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"strings"
	"time"
)

var (
	ErrContentDeliveryInvalid     = errors.New("invalid media content delivery command")
	ErrContentDeliveryConflict    = errors.New("media content delivery conflict")
	ErrContentDeliveryUnavailable = errors.New("media content delivery unavailable")
)

type ContentDeliveryStore interface {
	Eligible(context.Context, string, int64) (bool, error)
	Create(context.Context, mediaport.ContentPackageCommand, time.Time) (mediaport.ContentPackage, error)
	Update(context.Context, mediaport.ContentPackageUpdateCommand, time.Time) (mediaport.ContentPackage, error)
	Bind(context.Context, mediaport.DeliveryBindingCommand, time.Time) (mediaport.DeliveryBinding, error)
	GetBinding(context.Context, string, string) (mediaport.DeliveryBinding, error)
	Initiate(context.Context, mediaport.AttachmentUploadInitiateCommand, [32]byte, time.Time) (int64, error)
	PutPart(context.Context, mediaport.AttachmentUploadPartCommand, [32]byte, time.Time) (bool, error)
	Complete(context.Context, mediaport.AttachmentUploadCompleteCommand, time.Time) (int64, error)
}
type ContentDelivery struct {
	uow   platformport.UnitOfWork
	store ContentDeliveryStore
	now   func() time.Time
}

func NewContentDeliveryService(uow platformport.UnitOfWork, store ContentDeliveryStore) *ContentDelivery {
	return &ContentDelivery{uow: uow, store: store, now: time.Now}
}
func (s *ContentDelivery) Preview(ctx context.Context, c mediaport.ContentPackageCommand) (mediaport.ContentPackage, error) {
	if !validContent(c) || s == nil || s.store == nil {
		return mediaport.ContentPackage{}, ErrContentDeliveryInvalid
	}
	for _, r := range c.Refs {
		ok, e := s.store.Eligible(ctx, r.Kind, r.ID)
		if e != nil || !ok {
			return mediaport.ContentPackage{}, ErrContentDeliveryInvalid
		}
	}
	return mediaport.ContentPackage{Name: strings.TrimSpace(c.Name), ContentText: strings.TrimSpace(c.ContentText), Enabled: c.Enabled, Refs: append([]mediaport.ContentRef(nil), c.Refs...)}, nil
}
func (s *ContentDelivery) Create(ctx context.Context, c mediaport.ContentPackageCommand) (out mediaport.ContentPackage, err error) {
	if _, err = s.Preview(ctx, c); err != nil {
		return out, err
	}
	err = s.uow.Within(ctx, func(tx context.Context) error { out, err = s.store.Create(tx, c, s.now().UTC()); return err })
	if err != nil {
		return out, ErrContentDeliveryUnavailable
	}
	return out, nil
}
func (s *ContentDelivery) Update(ctx context.Context, c mediaport.ContentPackageUpdateCommand) (out mediaport.ContentPackage, err error) {
	if c.ID < 1 || c.ExpectedVersion < 1 {
		return out, ErrContentDeliveryInvalid
	}
	if _, err = s.Preview(ctx, c.ContentPackageCommand); err != nil {
		return out, err
	}
	err = s.uow.Within(ctx, func(tx context.Context) error { out, err = s.store.Update(tx, c, s.now().UTC()); return err })
	if err != nil {
		return out, ErrContentDeliveryConflict
	}
	return out, nil
}
func (s *ContentDelivery) Bind(ctx context.Context, c mediaport.DeliveryBindingCommand) (out mediaport.DeliveryBinding, err error) {
	if s == nil || s.store == nil || c.Actor < 1 || c.PackageID < 1 || c.GroupInviteID < 1 || c.CampaignCode == "" || c.PlanID == "" {
		return out, ErrContentDeliveryInvalid
	}
	err = s.uow.Within(ctx, func(tx context.Context) error { out, err = s.store.Bind(tx, c, s.now().UTC()); return err })
	if err != nil {
		return out, ErrContentDeliveryConflict
	}
	return out, nil
}
func (s *ContentDelivery) GetBinding(ctx context.Context, campaignCode, planID string) (out mediaport.DeliveryBinding, err error) {
	if s == nil || s.store == nil || campaignCode == "" || planID == "" {
		return out, ErrContentDeliveryInvalid
	}
	err = s.uow.Within(ctx, func(tx context.Context) error { out, err = s.store.GetBinding(tx, campaignCode, planID); return err })
	if err != nil {
		return out, ErrContentDeliveryUnavailable
	}
	return out, nil
}
func (s *ContentDelivery) InitiatePDF(ctx context.Context, c mediaport.AttachmentUploadInitiateCommand) (id int64, err error) {
	d, e := digest(c.SHA256)
	if e != nil || c.Size < 1 || c.Size > 10<<20 || c.Actor < 1 {
		return 0, ErrContentDeliveryInvalid
	}
	err = s.uow.Within(ctx, func(tx context.Context) error { id, e = s.store.Initiate(tx, c, d, s.now().UTC()); return e })
	if err != nil {
		return 0, ErrContentDeliveryUnavailable
	}
	return id, nil
}
func (s *ContentDelivery) PutPDFPart(ctx context.Context, c mediaport.AttachmentUploadPartCommand) error {
	d, e := digest(c.SHA256)
	if e != nil || c.UploadID < 1 || c.PartNumber < 1 || c.Actor < 1 || len(c.Content) < 1 {
		return ErrContentDeliveryInvalid
	}
	actual := sha256.Sum256(c.Content)
	if actual != d {
		return ErrContentDeliveryInvalid
	}
	return s.uow.Within(ctx, func(tx context.Context) error {
		ok, e := s.store.PutPart(tx, c, d, s.now().UTC())
		if e != nil || !ok {
			return ErrContentDeliveryConflict
		}
		return nil
	})
}
func (s *ContentDelivery) CompletePDF(ctx context.Context, c mediaport.AttachmentUploadCompleteCommand) (id int64, err error) {
	if c.UploadID < 1 || c.Actor < 1 {
		return 0, ErrContentDeliveryInvalid
	}
	err = s.uow.Within(ctx, func(tx context.Context) error { id, err = s.store.Complete(tx, c, s.now().UTC()); return err })
	if err != nil {
		return 0, ErrContentDeliveryUnavailable
	}
	return id, nil
}
func validContent(c mediaport.ContentPackageCommand) bool {
	return c.Actor > 0 && strings.TrimSpace(c.Name) != "" && strings.TrimSpace(c.Name) == c.Name && len(c.ContentText) <= 10000 && len(c.Refs) <= 100 && (strings.TrimSpace(c.ContentText) != "" || len(c.Refs) > 0)
}
func digest(v string) ([32]byte, error) {
	var r [32]byte
	if !strings.HasPrefix(v, "sha256:") {
		return r, errors.New("digest")
	}
	b, e := hex.DecodeString(strings.TrimPrefix(v, "sha256:"))
	if e != nil || len(b) != 32 {
		return r, errors.New("digest")
	}
	copy(r[:], b)
	return r, nil
}
