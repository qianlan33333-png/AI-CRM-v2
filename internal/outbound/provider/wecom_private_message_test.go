package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
)

type privateMessageClientFake struct {
	request privateMessageTemplateRequest
	result  privateMessageTemplateResult
	err     error
	calls   int
}

func (fake *privateMessageClientFake) CreatePrivateMessageTemplate(_ context.Context, request privateMessageTemplateRequest) (privateMessageTemplateResult, error) {
	fake.calls++
	fake.request = request
	return fake.result, fake.err
}

func TestWeComPrivateMessageProviderSendsExactTaskPayload(t *testing.T) {
	fake := &privateMessageClientFake{result: privateMessageTemplateResult{MessageID: "msg-1"}}
	provider, err := NewWeComPrivateMessageProvider(fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Send(context.Background(), privateMessageRequest(`{"sender":"staff-1","external_userid":"external-1","text":"hello"}`))
	if err != nil || result.MessageID != "msg-1" || !result.BusinessCallDispatched || !result.RealExternalCallExecuted || fake.calls != 1 || fake.request != (privateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: "hello"}) {
		t.Fatalf("result=%+v err=%v calls=%d request=%+v", result, err, fake.calls, fake.request)
	}
}

func TestWeComPrivateMessageProviderRejectsMissingRecipientBeforeClient(t *testing.T) {
	fake := &privateMessageClientFake{}
	provider, err := NewWeComPrivateMessageProvider(fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Send(context.Background(), privateMessageRequest(`{"sender":"staff-1","text":"hello"}`))
	if err != nil || result.FailureKind != outboundapp.ProviderFailureInvalidArgument || result.BusinessCallDispatched || result.RealExternalCallExecuted || fake.calls != 0 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, fake.calls)
	}
}

func TestWeComPrivateMessageProviderPreservesNoReplayOutcomes(t *testing.T) {
	for name, test := range map[string]struct {
		err            error
		kind           outboundapp.ProviderFailureKind
		code           string
		business, real bool
		received       bool
	}{
		"unknown":         {errWeComPrivateMessageOutcomeUnknown, outboundapp.ProviderFailureConnection, "wecom_write_outcome_unknown", true, true, false},
		"invalid result":  {errWeComPrivateMessageInvalidResult, outboundapp.ProviderFailureInvalidResult, "wecom_invalid_result", true, true, true},
		"target rejected": {errWeComPrivateMessageTargetRejected, outboundapp.ProviderFailureRecipientUnavailable, "wecom_private_target_rejected", true, true, true},
		"not dispatched":  {errWeComPrivateMessageNotDispatched, outboundapp.ProviderFailureTemporary, "wecom_not_dispatched", false, false, false},
		"throttled":       {fmt.Errorf("%w: %w", errWeComPrivateMessageUpstream, &weComPrivateMessageAPIError{Code: 45009}), outboundapp.ProviderFailureRateLimited, "wecom_errcode_45009", true, true, true},
		"contact missing": {fmt.Errorf("%w: %w", errWeComPrivateMessageUpstream, &weComPrivateMessageAPIError{Code: 84061}), outboundapp.ProviderFailureRecipientUnavailable, "wecom_errcode_84061", true, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &privateMessageClientFake{err: test.err}
			provider, err := NewWeComPrivateMessageProvider(fake, nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Send(context.Background(), privateMessageRequest(`{"sender":"staff-1","external_userid":"external-1","text":"hello"}`))
			if err != nil || result.FailureKind != test.kind || result.Code != test.code || result.ProviderResultReceived != test.received || result.BusinessCallDispatched != test.business || result.RealExternalCallExecuted != test.real || fake.calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, fake.calls)
			}
		})
	}
}

func TestWeComPrivateMessageProviderDoesNotHideUnexpectedClientErrors(t *testing.T) {
	fake := &privateMessageClientFake{err: errors.New("unexpected")}
	provider, err := NewWeComPrivateMessageProvider(fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Send(context.Background(), privateMessageRequest(`{"sender":"staff-1","external_userid":"external-1","text":"hello"}`))
	if err != nil || result.FailureKind != outboundapp.ProviderFailureInvalidResult || result.Code != "wecom_provider_error" || !result.BusinessCallDispatched || !result.RealExternalCallExecuted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestWeComPrivateMessageProviderResolvesOnlyWhenPayloadHasNoTarget(t *testing.T) {
	fake := &privateMessageClientFake{result: privateMessageTemplateResult{MessageID: "msg-1"}}
	calls := 0
	provider, err := NewWeComPrivateMessageProvider(fake, func(_ context.Context, customerID int64) (string, string, bool, error) {
		calls++
		if customerID != 1 {
			t.Fatalf("customerID=%d", customerID)
		}
		return "owner-1", "external-1", true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Send(context.Background(), privateMessageRequest(`{"text":"hello"}`))
	if err != nil || result.MessageID != "msg-1" || !result.BusinessCallDispatched || !result.RealExternalCallExecuted || calls != 1 || fake.request.Sender != "owner-1" || fake.request.ExternalUserID != "external-1" {
		t.Fatalf("result=%+v err=%v resolverCalls=%d request=%+v", result, err, calls, fake.request)
	}
}

func TestWeComPrivateMessageProviderFailsClosedForUnresolvedTarget(t *testing.T) {
	fake := &privateMessageClientFake{}
	provider, err := NewWeComPrivateMessageProvider(fake, func(context.Context, int64) (string, string, bool, error) {
		return "", "", false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Send(context.Background(), privateMessageRequest(`{"text":"hello"}`))
	if err != nil || result.FailureKind != outboundapp.ProviderFailureInvalidArgument || result.Code != "wecom_private_target_unavailable" || result.BusinessCallDispatched || result.RealExternalCallExecuted || fake.calls != 0 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, fake.calls)
	}
}

func TestWeComPrivateMessageProviderRetriesResolverFailureBeforeProviderCall(t *testing.T) {
	fake := &privateMessageClientFake{}
	provider, err := NewWeComPrivateMessageProvider(fake, func(context.Context, int64) (string, string, bool, error) {
		return "", "", false, errors.New("database unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Send(context.Background(), privateMessageRequest(`{"text":"hello"}`))
	if err != nil || result.FailureKind != outboundapp.ProviderFailureTemporary || result.Code != "wecom_private_target_resolution_unavailable" || result.BusinessCallDispatched || result.RealExternalCallExecuted || fake.calls != 0 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, fake.calls)
	}
}

func privateMessageRequest(payload string) outboundapp.SendRequest {
	return outboundapp.SendRequest{TaskID: 1, CustomerID: 1, TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: []byte(payload)}
}
