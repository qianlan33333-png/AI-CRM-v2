package worker

import (
	"context"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
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
