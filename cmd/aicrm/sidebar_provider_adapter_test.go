package main

import (
	"errors"
	"testing"

	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

func TestSidebarOAuthFailureFieldsExposeOnlySafeProviderCode(t *testing.T) {
	providerError := errors.Join(
		wecomclient.ErrUpstream,
		&wecomclient.APIError{Code: 50001, Message: "must-not-appear-in-log"},
	)
	category, code := sidebarOAuthFailureFields(providerError)
	if category != "upstream_rejected" || code != 50001 {
		t.Fatalf("fields = %q/%d", category, code)
	}

	for _, testCase := range []struct {
		err      error
		category string
	}{
		{wecomclient.ErrRequestTimeout, "timeout"},
		{wecomclient.ErrTransport, "transport"},
		{wecomclient.ErrUnexpectedResponse, "unexpected_response"},
		{errors.New("private failure detail"), "unknown"},
	} {
		category, code = sidebarOAuthFailureFields(testCase.err)
		if category != testCase.category || code != 0 {
			t.Fatalf("fields for %v = %q/%d", testCase.err, category, code)
		}
	}
}
