package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	eerport "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// CommerceExternalPushEERAccepter creates only a local EER acceptance fact.
// It never queues a job or invokes a Provider adapter.
type CommerceExternalPushEERAccepter struct{ runtime eerport.Runtime }

var _ productapp.ProductExternalPushEffectAccepter = (*CommerceExternalPushEERAccepter)(nil)

func NewCommerceExternalPushEERAccepter(runtime eerport.Runtime) *CommerceExternalPushEERAccepter {
	return &CommerceExternalPushEERAccepter{runtime: runtime}
}

func (a *CommerceExternalPushEERAccepter) AcceptProductExternalPushTest(ctx context.Context, command productapp.ProductExternalPushEffectCommand) (productport.ExternalPushTest, error) {
	if a == nil || a.runtime == nil || command.ProductID < 1 || !validExternalPushKind(command.ProductKind) {
		return productport.ExternalPushTest{}, productapp.ErrUnavailable
	}
	envelope, err := eerport.NewEnvelope(eerport.EnvelopeInput{
		Owner: eerport.OwnerProduct, Kind: eerport.KindProductExternalPushTest,
		SourceRefDigest:   productExternalPushDigest("source", strconv.FormatInt(int64(command.ProductID), 10), string(command.ProductKind)),
		TargetRefDigest:   productExternalPushDigest("target", hex.EncodeToString(command.ConfigurationDigest[:])),
		PayloadDigest:     productExternalPushDigest("payload", hex.EncodeToString(command.ConfigurationDigest[:])),
		PolicyVersionHash: productExternalPushDigest("policy", "commerce-external-push/v1"),
	})
	if err != nil {
		return productport.ExternalPushTest{}, productapp.ErrUnavailable
	}
	projection, _, err := a.runtime.Accept(ctx, eerport.AcceptCommand{
		ReceiptKeyDigest: productExternalPushDigest("receipt", hex.EncodeToString(command.ReceiptKeyDigest[:])),
		Envelope:         envelope,
	})
	if err != nil || projection.ID == "" || projection.State != eerport.StateAccepted || projection.Owner != eerport.OwnerProduct ||
		projection.Kind != eerport.KindProductExternalPushTest || projection.UpdatedAt.IsZero() {
		return productport.ExternalPushTest{}, productapp.ErrUnavailable
	}
	return productport.ExternalPushTest{
		ProductID: command.ProductID, ProductKind: command.ProductKind, EffectID: projection.ID,
		State: string(projection.State), CreatedAt: projection.UpdatedAt,
	}, nil
}

func productExternalPushDigest(parts ...string) eerport.Digest {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return eerport.Digest("sha256:" + hex.EncodeToString(hash.Sum(nil)))
}
