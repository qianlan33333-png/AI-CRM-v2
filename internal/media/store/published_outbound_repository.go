package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	"strconv"
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
	mediaRefs := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		if !ref.RefID.Valid || ref.RefID.Int64 < 1 {
			return mediaapp.PublishedPackage{}, mediaapp.ErrPublishedOutboundInvalid
		}
		h.Write([]byte(ref.RefKind))
		h.Write([]byte("\x00"))
		h.Write([]byte(strconv.FormatInt(ref.RefID.Int64, 10)))
		mediaRefs = append(mediaRefs, map[string]any{"kind": ref.RefKind, "id": ref.RefID.Int64})
	}
	rawRefs, e := json.Marshal(mediaRefs)
	if e != nil {
		return mediaapp.PublishedPackage{}, e
	}
	return mediaapp.PublishedPackage{ID: p.ID, Enabled: p.Enabled, Snapshot: hex.EncodeToString(h.Sum(nil)), MediaRefs: rawRefs}, nil
}
func (r *PublishedOutboundRepository) BindOutboundMediaAcceptance(ctx context.Context, value mediaapp.PublishedOutboundAcceptance) (bool, error) {
	binding, err := r.content.BindOutboundMediaEffect(ctx, value.ContentPackageID, value.TargetDigest, value.SourceDigest, value.EffectID, value.CreatedAt)
	if err != nil {
		return false, err
	}
	q, err := contentQueries(ctx)
	if err != nil {
		return false, err
	}
	params := mediadb.InsertOutboundMediaAcceptanceParams{ContentPackageID: value.ContentPackageID, TargetDigest: value.TargetDigest, MediaRefs: value.MediaRefs, SourceDigest: value.SourceDigest, PayloadDigest: value.PayloadDigest, ExternalEffectID: value.EffectID, CreatedAt: stamp(value.CreatedAt)}
	row, err := q.InsertOutboundMediaAcceptance(ctx, params)
	if err == nil {
		if binding.Replay {
			return false, mediaapp.ErrPublishedOutboundInvalid
		}
		return false, validateOutboundMediaAcceptance(row.ContentPackageID, row.TargetDigest, row.MediaRefs, row.SourceDigest, row.PayloadDigest, row.ExternalEffectID, row.State, value)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	existing, err := q.GetOutboundMediaAcceptance(ctx, mediadb.GetOutboundMediaAcceptanceParams{ContentPackageID: value.ContentPackageID, TargetDigest: value.TargetDigest})
	if err != nil {
		return false, err
	}
	if !binding.Replay {
		return false, mediaapp.ErrPublishedOutboundInvalid
	}
	return true, validateOutboundMediaAcceptance(existing.ContentPackageID, existing.TargetDigest, existing.MediaRefs, existing.SourceDigest, existing.PayloadDigest, existing.ExternalEffectID, existing.State, value)
}

func validateOutboundMediaAcceptance(packageID int64, target string, refs []byte, source, payload string, effectID int64, state string, want mediaapp.PublishedOutboundAcceptance) error {
	if packageID != want.ContentPackageID || target != want.TargetDigest || source != want.SourceDigest || payload != want.PayloadDigest || effectID != want.EffectID || state != "accepted" || !equalJSONBytes(refs, want.MediaRefs) {
		return mediaapp.ErrPublishedOutboundInvalid
	}
	return nil
}

func equalJSONBytes(left, right []byte) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return bytes.Equal(aJSON, bJSON)
}
