package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

var ErrCampaignWeComAdapter = errors.New("invalid outbound campaign WeCom adapter")

// CampaignWeComAdapter is the only bridge from a C01 effect to the existing
// approved WeCom private-message Provider. It resolves the private content
// only after EER persisted the attempt fence; EER itself remains digest-only.
type CampaignWeComAdapter struct {
	loader   campaignDispatchProviderRequestLoader
	provider outboundapp.ProviderAdapter
}

var _ eer.Adapter = (*CampaignWeComAdapter)(nil)

type campaignDispatchProviderRequestLoader interface {
	LoadCampaignDispatchProviderRequest(context.Context, string) (outboundport.CampaignDispatchProviderRequest, error)
}

func NewCampaignWeComAdapter(loader campaignDispatchProviderRequestLoader, provider outboundapp.ProviderAdapter) (*CampaignWeComAdapter, error) {
	if loader == nil || provider == nil {
		return nil, ErrCampaignWeComAdapter
	}
	return &CampaignWeComAdapter{loader: loader, provider: provider}, nil
}

func (adapter *CampaignWeComAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	if ctx == nil || adapter == nil || adapter.loader == nil || adapter.provider == nil || envelope.Owner() != eer.OwnerOutbound || envelope.Kind() != eer.KindOutboundMessage || attempt.Number < 1 {
		return eer.AdapterResult{}, ErrCampaignWeComAdapter
	}
	request, err := adapter.loader.LoadCampaignDispatchProviderRequest(ctx, string(envelope.PayloadDigest()))
	if err != nil || request.DispatchID < 1 || request.CustomerID < 1 || request.StepIndex < 1 || strings.TrimSpace(request.Content) == "" || request.PayloadDigest != string(envelope.PayloadDigest()) {
		return eer.AdapterResult{}, errors.Join(ErrCampaignWeComAdapter, err)
	}
	payload, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: request.Content})
	if err != nil {
		return eer.AdapterResult{}, errors.Join(ErrCampaignWeComAdapter, err)
	}
	result, err := adapter.provider.Send(ctx, outboundapp.SendRequest{TaskID: outboundapp.TaskID(request.DispatchID), CustomerID: request.CustomerID, TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: payload})
	if err != nil {
		return eer.AdapterResult{}, err
	}
	return campaignWeComResult(envelope, attempt, result), nil
}

func campaignWeComResult(envelope eer.EffectEnvelope, attempt eer.Attempt, result outboundapp.ProviderResult) eer.AdapterResult {
	completion := eer.CompletionOutcomeUnknown
	if result.FailureKind == "" && strings.TrimSpace(result.MessageID) != "" {
		completion = eer.CompletionExecuted
	} else {
		switch result.FailureKind {
		case outboundapp.ProviderFailureRateLimited, outboundapp.ProviderFailureTemporary:
			completion = eer.Completion("retryable_failed")
		case outboundapp.ProviderFailureInvalidArgument, outboundapp.ProviderFailureRecipientUnavailable:
			completion = eer.CompletionFinalFailed
		}
	}
	digest := campaignWeComReceiptDigest(envelope, attempt, result)
	return eer.AdapterResult{Completion: completion, ReceiptDigest: digest, ResultReferenceDigest: digest}
}

func campaignWeComReceiptDigest(envelope eer.EffectEnvelope, attempt eer.Attempt, result outboundapp.ProviderResult) eer.Digest {
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.wecom.v1\x00" + string(envelope.Fingerprint()) + "\x00" + strconv.FormatInt(int64(attempt.Number), 10) + "\x00" + string(result.FailureKind) + "\x00" + result.Code + "\x00" + result.MessageID))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
