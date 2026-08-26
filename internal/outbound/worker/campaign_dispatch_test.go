package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type providerShapeSpy struct{ calls int }

func (spy *providerShapeSpy) Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
	spy.calls++
	return eer.AdapterResult{}, errors.New("must not call")
}

func TestProviderShapedAdapterDisabledNeverCallsProvider(t *testing.T) {
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: workerDigest("source"), TargetRefDigest: workerDigest("target"), PayloadDigest: workerDigest("payload"), PolicyVersionHash: workerDigest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	spy := &providerShapeSpy{}
	result, err := (ProviderShapedAdapter{Provider: spy}).Execute(context.Background(), envelope, eer.Attempt{Number: 1})
	if err != nil || result.Completion != eer.CompletionFinalFailed || result.ReceiptDigest == "" || spy.calls != 0 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, spy.calls)
	}
}

func workerDigest(string) eer.Digest {
	return eer.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
}

type campaignWeComLoaderSpy struct {
	request outboundport.CampaignDispatchProviderRequest
	digest  string
	err     error
}

func (spy *campaignWeComLoaderSpy) LoadCampaignDispatchProviderRequest(_ context.Context, digest string) (outboundport.CampaignDispatchProviderRequest, error) {
	spy.digest = digest
	return spy.request, spy.err
}

type campaignWeComProviderSpy struct {
	request outboundapp.SendRequest
	result  outboundapp.ProviderResult
	err     error
	calls   int
}

func (spy *campaignWeComProviderSpy) Send(_ context.Context, request outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	spy.calls++
	spy.request = request
	return spy.result, spy.err
}

func TestCampaignWeComAdapterResolvesPrivateRequestAfterAttemptFence(t *testing.T) {
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: workerDigest("source"), TargetRefDigest: workerDigest("target"), PayloadDigest: workerDigest("payload"), PolicyVersionHash: workerDigest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	loader := &campaignWeComLoaderSpy{request: outboundport.CampaignDispatchProviderRequest{DispatchID: 41, HandoffID: 19, CustomerID: 7, StepIndex: 2, Content: "hello", PayloadDigest: string(envelope.PayloadDigest())}}
	provider := &campaignWeComProviderSpy{result: outboundapp.ProviderResult{MessageID: "provider-message-id"}}
	adapter, err := NewCampaignWeComAdapter(loader, provider)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), envelope, eer.Attempt{Number: 1, Generation: 2, Fence: 3})
	if err != nil || result.Completion != eer.CompletionExecuted || result.ReceiptDigest == "" || loader.digest != string(envelope.PayloadDigest()) || provider.calls != 1 || provider.request.TaskID != 41 || provider.request.CustomerID != 7 || provider.request.TemplateKey != outboundapp.TemplateTextNoticeV1 {
		t.Fatalf("result=%+v err=%v loader=%+v provider=%+v", result, err, loader, provider)
	}
	var payload map[string]string
	if err = json.Unmarshal(provider.request.Payload, &payload); err != nil || len(payload) != 1 || payload["text"] != "hello" {
		t.Fatalf("payload=%s err=%v", provider.request.Payload, err)
	}
}

func TestCampaignWeComAdapterFailsClosedBeforeProviderForMismatchedSnapshot(t *testing.T) {
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: workerDigest("source"), TargetRefDigest: workerDigest("target"), PayloadDigest: workerDigest("payload"), PolicyVersionHash: workerDigest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	loader := &campaignWeComLoaderSpy{request: outboundport.CampaignDispatchProviderRequest{DispatchID: 41, CustomerID: 7, StepIndex: 1, Content: "hello", PayloadDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000001"}}
	provider := &campaignWeComProviderSpy{}
	adapter, err := NewCampaignWeComAdapter(loader, provider)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.Execute(context.Background(), envelope, eer.Attempt{Number: 1}); !errors.Is(err, ErrCampaignWeComAdapter) || provider.calls != 0 {
		t.Fatalf("err=%v provider calls=%d", err, provider.calls)
	}
}

func TestCampaignWeComAdapterClassifiesProviderTerminalStates(t *testing.T) {
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: workerDigest("source"), TargetRefDigest: workerDigest("target"), PayloadDigest: workerDigest("payload"), PolicyVersionHash: workerDigest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		in   outboundapp.ProviderResult
		want eer.Completion
	}{
		{name: "retryable", in: outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureTemporary, Code: "temporary"}, want: eer.Completion("retryable_failed")},
		{name: "final", in: outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureRecipientUnavailable, Code: "84061"}, want: eer.CompletionFinalFailed},
		{name: "unknown", in: outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureConnection, Code: "unknown"}, want: eer.CompletionOutcomeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := campaignWeComResult(envelope, eer.Attempt{Number: 1, Generation: 2, Fence: 3}, test.in)
			if result.Completion != test.want || result.ReceiptDigest == "" || result.BusinessCallDispatched || result.RealExternalCallExecuted {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}
