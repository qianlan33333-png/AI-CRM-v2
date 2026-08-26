package wecom

import (
	"context"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

func TestAudienceDispatchReconciliationVerifierRequiresExactSingleRecipientEvidence(t *testing.T) {
	evidence := outboundport.CampaignDispatchReconciliationEvidence{
		ExternalEffectID: "eer_71", ProviderMessageID: "msg-71", SenderUserID: "owner-71", ExternalUserID: "external-71",
		ProviderReceiptDigest: eer.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), BusinessCallDispatched: true, RealExternalCallExecuted: true,
	}
	for _, test := range []struct {
		name    string
		result  GroupMessageSendResult
		deliver bool
	}{
		{name: "exact status one", result: GroupMessageSendResult{ChatID: "chat-71", ExternalUserID: "external-71", UserID: "owner-71", Status: 1}, deliver: true},
		{name: "different recipient", result: GroupMessageSendResult{ChatID: "chat-71", ExternalUserID: "external-72", UserID: "owner-71", Status: 1}},
		{name: "different owner", result: GroupMessageSendResult{ChatID: "chat-71", ExternalUserID: "external-71", UserID: "owner-72", Status: 1}},
		{name: "not sent", result: GroupMessageSendResult{ChatID: "chat-71", ExternalUserID: "external-71", UserID: "owner-71", Status: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier, err := NewAudienceDispatchReconciliationVerifier(audienceDispatchReadFake{result: test.result})
			if err != nil {
				t.Fatal(err)
			}
			delivery, digest, err := verifier.VerifyAudienceCampaignDispatch(context.Background(), evidence)
			if err != nil || delivery != test.deliver || !validGroupMessageDigest(string(digest)) {
				t.Fatalf("delivery=%t digest=%q err=%v", delivery, digest, err)
			}
		})
	}
}

func TestAudienceDispatchReconciliationVerifierStopsOnQueryFailure(t *testing.T) {
	evidence := outboundport.CampaignDispatchReconciliationEvidence{ExternalEffectID: "eer_71", ProviderMessageID: "msg-71", SenderUserID: "owner-71", ExternalUserID: "external-71", ProviderReceiptDigest: eer.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), BusinessCallDispatched: true, RealExternalCallExecuted: true}
	verifier, err := NewAudienceDispatchReconciliationVerifier(audienceDispatchReadFake{err: errors.New("upstream unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = verifier.VerifyAudienceCampaignDispatch(context.Background(), evidence); err == nil {
		t.Fatal("VerifyAudienceCampaignDispatch() error=nil")
	}
}

type audienceDispatchReadFake struct {
	result GroupMessageSendResult
	err    error
}

func (fake audienceDispatchReadFake) GetGroupMessageTask(_ context.Context, messageID, cursor string) (GroupMessageTaskPage, error) {
	if fake.err != nil || messageID != "msg-71" || cursor != "" {
		return GroupMessageTaskPage{}, fake.err
	}
	return GroupMessageTaskPage{Items: []GroupMessageTask{{UserID: "owner-71", Status: 2}}}, nil
}

func (fake audienceDispatchReadFake) GetGroupMessageSendResult(_ context.Context, messageID, userID, cursor string) (GroupMessageSendResultPage, error) {
	if fake.err != nil || messageID != "msg-71" || userID != "owner-71" || cursor != "" {
		return GroupMessageSendResultPage{}, fake.err
	}
	return GroupMessageSendResultPage{Items: []GroupMessageSendResult{fake.result}}, nil
}
