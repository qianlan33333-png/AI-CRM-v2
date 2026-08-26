package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

var ErrInvalidAudienceDispatchReconciliation = errors.New("invalid WeCom Audience dispatch reconciliation")

type audienceDispatchGroupMessageClient interface {
	GetGroupMessageTask(context.Context, string, string) (GroupMessageTaskPage, error)
	GetGroupMessageSendResult(context.Context, string, string, string) (GroupMessageSendResultPage, error)
}

// AudienceDispatchReconciliationVerifier proves single-recipient delivery only
// from the independent WeCom result endpoint: exact msgid, frozen owner userid,
// frozen external_userid and status=1. Missing records deliberately remain an
// observed-not-delivered result, not a failure claim.
type AudienceDispatchReconciliationVerifier struct {
	client audienceDispatchGroupMessageClient
}

var _ outboundport.CampaignDispatchReconciliationEvidenceVerifier = (*AudienceDispatchReconciliationVerifier)(nil)

func NewAudienceDispatchReconciliationVerifier(client audienceDispatchGroupMessageClient) (*AudienceDispatchReconciliationVerifier, error) {
	if client == nil {
		return nil, ErrInvalidAudienceDispatchReconciliation
	}
	return &AudienceDispatchReconciliationVerifier{client: client}, nil
}

func (verifier *AudienceDispatchReconciliationVerifier) VerifyAudienceCampaignDispatch(ctx context.Context, evidence outboundport.CampaignDispatchReconciliationEvidence) (bool, eer.Digest, error) {
	if verifier == nil || verifier.client == nil || ctx == nil || evidence.ExternalEffectID == "" || !evidence.BusinessCallDispatched || !evidence.RealExternalCallExecuted || !validGroupMessageReadText(evidence.ProviderMessageID, 1024) || !validGroupMessageReadText(evidence.SenderUserID, 128) || !validGroupMessageReadText(evidence.ExternalUserID, 1024) || !validGroupMessageDigest(string(evidence.ProviderReceiptDigest)) {
		return false, "", ErrInvalidAudienceDispatchReconciliation
	}
	if _, err := verifier.client.GetGroupMessageTask(ctx, evidence.ProviderMessageID, ""); err != nil {
		return false, "", err
	}
	seen := map[string]struct{}{}
	for cursor := ""; ; {
		if _, duplicate := seen[cursor]; duplicate {
			return false, "", errGroupMessageReadUnknown
		}
		seen[cursor] = struct{}{}
		page, err := verifier.client.GetGroupMessageSendResult(ctx, evidence.ProviderMessageID, evidence.SenderUserID, cursor)
		if err != nil {
			return false, "", err
		}
		for _, item := range page.Items {
			if item.ExternalUserID == evidence.ExternalUserID && item.UserID == evidence.SenderUserID && item.Status == 1 {
				return true, audienceDispatchReconciliationDigest("delivery", evidence, "1"), nil
			}
		}
		if page.NextCursor == "" {
			return false, audienceDispatchReconciliationDigest("not_observed", evidence, ""), nil
		}
		cursor = page.NextCursor
	}
}

func audienceDispatchReconciliationDigest(disposition string, evidence outboundport.CampaignDispatchReconciliationEvidence, status string) eer.Digest {
	sum := sha256.Sum256([]byte("wecom.audience_dispatch.reconcile.v1\x00" + disposition + "\x00" + evidence.ExternalEffectID + "\x00" + evidence.ProviderMessageID + "\x00" + evidence.SenderUserID + "\x00" + evidence.ExternalUserID + "\x00" + string(evidence.ProviderReceiptDigest) + "\x00" + status))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
