package app

import (
	"context"
	"encoding/json"
	"errors"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"strconv"
	"strings"
	"time"
)

var ErrPublishedOutboundInvalid = errors.New("invalid published content package outbound")

type PublishedPackage struct {
	ID        int64
	Enabled   bool
	Snapshot  string
	MediaRefs json.RawMessage
}
type PublishedPackageReader interface {
	ReadPublishedPackage(context.Context, int64) (PublishedPackage, error)
}
type OutboundEffectBinder interface {
	BindOutboundMediaAcceptance(context.Context, PublishedOutboundAcceptance) (bool, error)
}
type PublishedOutboundAcceptance struct {
	ContentPackageID int64
	TargetDigest     string
	SourceDigest     string
	PayloadDigest    string
	MediaRefs        json.RawMessage
	EffectID         int64
	CreatedAt        time.Time
}
type PublishedOutboundService struct {
	uow    platformport.UnitOfWork
	reader PublishedPackageReader
	accept *OutboundMediaService
	binder OutboundEffectBinder
	now    func() time.Time
}

func NewPublishedOutboundService(uow platformport.UnitOfWork, r PublishedPackageReader, a *OutboundMediaService, b OutboundEffectBinder) *PublishedOutboundService {
	return &PublishedOutboundService{uow: uow, reader: r, accept: a, binder: b, now: time.Now}
}
func (s *PublishedOutboundService) AcceptPublishedContentPackageForOutbound(ctx context.Context, packageID int64, targetRef, idem string) (eer.Projection, bool, error) {
	if s == nil || s.uow == nil || s.reader == nil || s.accept == nil || s.binder == nil || packageID < 1 || strings.TrimSpace(targetRef) == "" || len(idem) < 16 || len(idem) > 128 {
		return eer.Projection{}, false, ErrPublishedOutboundInvalid
	}
	var effect eer.Projection
	var replay bool
	err := s.within(ctx, func(tx context.Context) error {
		p, err := s.reader.ReadPublishedPackage(tx, packageID)
		if err != nil || !p.Enabled || p.ID != packageID || p.Snapshot == "" || !json.Valid(p.MediaRefs) {
			return ErrPublishedOutboundInvalid
		}
		target := mediaEERDigest("outbound-media-target", targetRef)
		snapshot := mediaEERDigest("outbound-media-snapshot", p.Snapshot)
		payload := mediaEERDigest("outbound-media-payload", p.Snapshot, target)
		effect, err = s.accept.AcceptOutboundMedia(tx, OutboundMediaAcceptCommand{SourceDigest: snapshot, TargetDigest: target, PayloadDigest: payload, ReceiptKey: idem})
		if err != nil {
			return err
		}
		id, err := decodeEERID(effect.ID)
		if err != nil {
			return err
		}
		replay, err = s.binder.BindOutboundMediaAcceptance(tx, PublishedOutboundAcceptance{ContentPackageID: packageID, TargetDigest: target, SourceDigest: snapshot, PayloadDigest: payload, MediaRefs: append(json.RawMessage(nil), p.MediaRefs...), EffectID: id, CreatedAt: s.now().UTC()})
		return err
	})
	if err != nil {
		return eer.Projection{}, false, err
	}
	return effect, replay, nil
}

func (s *PublishedOutboundService) within(ctx context.Context, fn func(context.Context) error) error {
	if _, err := platformstore.TxFromContext(ctx); err == nil {
		return fn(ctx)
	}
	return s.uow.Within(ctx, fn)
}
func decodeEERID(v string) (int64, error) {
	if !strings.HasPrefix(v, "eer_") {
		return 0, ErrPublishedOutboundInvalid
	}
	id, e := strconv.ParseInt(strings.TrimPrefix(v, "eer_"), 10, 64)
	if e != nil || id < 1 {
		return 0, ErrPublishedOutboundInvalid
	}
	return id, nil
}
