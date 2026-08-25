package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	"strings"
)

var ErrOutboundMediaInvalid = errors.New("invalid outbound media acceptance")

type OutboundMediaAcceptCommand struct{ SourceDigest, TargetDigest, PayloadDigest, ReceiptKey string }
type OutboundMediaRuntime interface {
	Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error)
}
type OutboundMediaService struct{ runtime OutboundMediaRuntime }

func NewOutboundMediaService(runtime OutboundMediaRuntime) *OutboundMediaService {
	return &OutboundMediaService{runtime: runtime}
}
func (s *OutboundMediaService) AcceptOutboundMedia(ctx context.Context, c OutboundMediaAcceptCommand) (eer.Projection, error) {
	if s == nil || s.runtime == nil || ctx == nil || !validMediaDigest(c.SourceDigest) || !validMediaDigest(c.TargetDigest) || !validMediaDigest(c.PayloadDigest) || strings.TrimSpace(c.ReceiptKey) == "" {
		return eer.Projection{}, ErrOutboundMediaInvalid
	}
	policy := mediaEERDigest("outbound-media-policy-v1")
	envelope, e := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMedia, SourceRefDigest: eer.Digest(c.SourceDigest), TargetRefDigest: eer.Digest(c.TargetDigest), PayloadDigest: eer.Digest(c.PayloadDigest), PolicyVersionHash: eer.Digest(policy)})
	if e != nil {
		return eer.Projection{}, ErrOutboundMediaInvalid
	}
	p, _, e := s.runtime.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: eer.Digest(mediaEERDigest("outbound-media-receipt", c.ReceiptKey)), Envelope: envelope})
	if e != nil {
		return eer.Projection{}, e
	}
	if p.Owner != eer.OwnerOutbound || p.Kind != eer.KindOutboundMedia || p.State != eer.StateAccepted {
		return eer.Projection{}, ErrOutboundMediaInvalid
	}
	return p, nil
}
func mediaEERDigest(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(h[:])
}
func validMediaDigest(v string) bool {
	if !strings.HasPrefix(v, "sha256:") || len(v) != 71 {
		return false
	}
	_, e := hex.DecodeString(v[7:])
	return e == nil
}
