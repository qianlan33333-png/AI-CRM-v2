package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"strconv"
	"time"
)

type PublishedOutboundRepository struct{ content *ContentDeliveryRepository }

func NewPublishedOutboundRepository() *PublishedOutboundRepository {
	return &PublishedOutboundRepository{content: NewContentDeliveryRepository()}
}
func (r *PublishedOutboundRepository) ReadPublishedPackage(ctx context.Context, id int64) (mediaapp.PublishedPackage, error) {
	q, e := contentQueries(ctx)
	if e != nil {
		return mediaapp.PublishedPackage{}, e
	}
	p, e := q.GetMediaContentPackage(ctx, id)
	if e != nil {
		return mediaapp.PublishedPackage{}, e
	}
	refs, e := q.ListMediaContentPackageRefs(ctx, id)
	if e != nil {
		return mediaapp.PublishedPackage{}, e
	}
	h := sha256.New()
	h.Write([]byte(p.Name + "\x00" + p.ContentText))
	for _, ref := range refs {
		h.Write([]byte(ref.RefKind))
		h.Write([]byte("\x00"))
		h.Write([]byte(strconv.FormatInt(ref.RefID.Int64, 10)))
	}
	return mediaapp.PublishedPackage{ID: p.ID, Enabled: p.Enabled, Snapshot: hex.EncodeToString(h.Sum(nil))}, nil
}
func (r *PublishedOutboundRepository) BindOutboundMediaEffect(ctx context.Context, p int64, t, s string, effect int64, now time.Time) (bool, error) {
	v, e := r.content.BindOutboundMediaEffect(ctx, p, t, s, effect, now)
	return v.Replay, e
}
