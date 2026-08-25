package app

import (
	"context"
	"errors"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	"strconv"
	"strings"
	"time"
)

var ErrPublishedOutboundInvalid = errors.New("invalid published content package outbound")

type PublishedPackage struct {
	ID       int64
	Enabled  bool
	Snapshot string
}
type PublishedPackageReader interface {
	ReadPublishedPackage(context.Context, int64) (PublishedPackage, error)
}
type OutboundEffectBinder interface {
	BindOutboundMediaEffect(context.Context, int64, string, string, int64, time.Time) (bool, error)
}
type PublishedOutboundService struct {
	reader PublishedPackageReader
	accept *OutboundMediaService
	binder OutboundEffectBinder
	now    func() time.Time
}

func NewPublishedOutboundService(r PublishedPackageReader, a *OutboundMediaService, b OutboundEffectBinder) *PublishedOutboundService {
	return &PublishedOutboundService{reader: r, accept: a, binder: b, now: time.Now}
}
func (s *PublishedOutboundService) AcceptPublishedContentPackageForOutbound(ctx context.Context, packageID int64, targetRef, idem string) (eer.Projection, bool, error) {
	if s == nil || s.reader == nil || s.accept == nil || s.binder == nil || packageID < 1 || strings.TrimSpace(targetRef) == "" || strings.TrimSpace(idem) == "" {
		return eer.Projection{}, false, ErrPublishedOutboundInvalid
	}
	p, e := s.reader.ReadPublishedPackage(ctx, packageID)
	if e != nil || !p.Enabled || p.ID != packageID || p.Snapshot == "" {
		return eer.Projection{}, false, ErrPublishedOutboundInvalid
	}
	target := mediaEERDigest("outbound-media-target", targetRef)
	snapshot := mediaEERDigest("outbound-media-snapshot", p.Snapshot)
	payload := mediaEERDigest("outbound-media-payload", p.Snapshot, target)
	effect, e := s.accept.AcceptOutboundMedia(ctx, OutboundMediaAcceptCommand{SourceDigest: snapshot, TargetDigest: target, PayloadDigest: payload, ReceiptKey: idem})
	if e != nil {
		return eer.Projection{}, false, e
	}
	id, e := decodeEERID(effect.ID)
	if e != nil {
		return eer.Projection{}, false, e
	}
	replay, e := s.binder.BindOutboundMediaEffect(ctx, packageID, target, payload, id, s.now().UTC())
	if e != nil {
		return eer.Projection{}, false, e
	}
	return effect, replay, nil
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
