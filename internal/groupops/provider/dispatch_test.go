package groupopsprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestDispatchAdapterClassifiesProviderBoundaryWithoutDeliveryClaim(t *testing.T) {
	request := testExecution()
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, SourceRefDigest: digest("source"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		result   groupopsport.DispatchProviderResult
		err      error
		want     eer.Completion
		business bool
		real     bool
	}{
		{name: "pre dispatch", result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchPreDispatchFailure}, want: eer.CompletionFinalFailed},
		{name: "accepted", result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: string(digest("accepted")), BusinessCallDispatched: true, RealExternalCallExecuted: true}, want: eer.CompletionExecuted, business: true, real: true},
		{name: "unknown", result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchOutcomeUnknown, BusinessCallDispatched: true, RealExternalCallExecuted: true}, want: eer.CompletionOutcomeUnknown, business: true, real: true},
		{name: "rejected", result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderRejected, ReceiptDigest: string(digest("rejected")), BusinessCallDispatched: true, RealExternalCallExecuted: true}, want: eer.CompletionFinalFailed, business: true, real: true},
		{name: "boundary error", result: groupopsport.DispatchProviderResult{BusinessCallDispatched: true, RealExternalCallExecuted: true}, err: errors.New("controlled transport interruption"), business: true, real: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &providerStub{result: test.result, err: test.err}
			adapter, err := NewDispatchAdapter(provider, request)
			if err != nil {
				t.Fatal(err)
			}
			result, err := adapter.Execute(context.Background(), envelope, eer.Attempt{Number: 1, Generation: 1, Fence: 1})
			if !errors.Is(err, test.err) || result.Completion != test.want || result.BusinessCallDispatched != test.business || result.RealExternalCallExecuted != test.real || provider.calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, provider.calls)
			}
			if result.Completion == eer.CompletionExecuted && result.ReceiptDigest == "" {
				t.Fatalf("accepted result missing receipt: %+v", result)
			}
		})
	}
}

func TestDispatchAdapterRejectsImplicitRealExternalCall(t *testing.T) {
	adapter, err := NewDispatchAdapter(&providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: string(digest("accepted"))}}, testExecution())
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, SourceRefDigest: digest("source"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), envelope, eer.Attempt{Number: 1, Generation: 1, Fence: 1})
	if !errors.Is(err, ErrInvalidDispatch) || result.BusinessCallDispatched || result.RealExternalCallExecuted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestDispatchAdapterRejectsInvalidExecutionBeforeProviderBoundary(t *testing.T) {
	invalid := testExecution()
	invalid.ContentDigest = "not-a-digest"
	provider := &providerStub{}
	if _, err := NewDispatchAdapter(provider, invalid); !errors.Is(err, ErrInvalidDispatch) || provider.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, provider.calls)
	}
}

func TestDispatchAdapterNilIsSafePreDispatchFailure(t *testing.T) {
	var adapter *DispatchAdapter
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, SourceRefDigest: digest("source"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), envelope, eer.Attempt{})
	if err != nil || result.Completion != eer.CompletionFinalFailed || result.BusinessCallDispatched || result.RealExternalCallExecuted || result.ReceiptDigest == "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type providerStub struct {
	result groupopsport.DispatchProviderResult
	err    error
	calls  int
}

func (stub *providerStub) Dispatch(_ context.Context, request groupopsport.DispatchRequest) (groupopsport.DispatchProviderResult, error) {
	stub.calls++
	if request.ExecutionID != 11 || request.ExternalEffectID != "eer_41" || string(request.ContentSnapshot) != `{"schema_version":1}` {
		return groupopsport.DispatchProviderResult{}, errors.New("unexpected controlled request")
	}
	return stub.result, stub.err
}

func testExecution() groupopsport.DispatchExecution {
	return groupopsport.DispatchExecution{ExecutionID: 11, ExternalEffectID: "eer_41", State: groupopsport.ExecutionAccepted, TargetReference: "chat:controlled", SenderUserID: "staff-1", ContentSnapshot: []byte(`{"schema_version":1}`), ContentDigest: string(digest("content")), MaterialSnapshot: []byte(`{"schema_version":1}`), MaterialDigest: string(digest("material"))}
}

func digest(value string) eer.Digest {
	sum := sha256.Sum256([]byte(value))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
