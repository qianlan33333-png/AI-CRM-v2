package client

import (
	"errors"
	"testing"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestCustomerAcquisitionLinkProviderClassifiesWriteBoundary(t *testing.T) {
	input := CustomerAcquisitionLinkRequest{LinkName: "获客链接", UserIDs: []string{"staff-a"}}
	link := CustomerAcquisitionLink{LinkID: "link-1", LinkName: input.LinkName, URL: "https://work.weixin.qq.com/ca/link-1", UserIDs: input.UserIDs}
	for _, testCase := range []struct {
		name           string
		err            error
		wantOutcome    wecomport.CustomerAcquisitionLinkWriteOutcome
		wantDispatched bool
		wantReal       bool
		wantError      error
	}{
		{name: "executed", wantOutcome: wecomport.CustomerAcquisitionLinkExecuted, wantDispatched: true, wantReal: true},
		{name: "provider rejected", err: apiError(40058, "rejected"), wantOutcome: wecomport.CustomerAcquisitionLinkFinalFailed, wantDispatched: true, wantReal: true},
		{name: "outcome unknown", err: ErrWriteOutcomeUnknown, wantOutcome: wecomport.CustomerAcquisitionLinkOutcomeUnknown, wantDispatched: true, wantReal: true},
		{name: "not dispatched", err: ErrBusinessWriteNotDispatched, wantError: wecomport.ErrCustomerAcquisitionLinkNotDispatched},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := customerAcquisitionLinkWriteResult("create", "", input, link, testCase.err)
			if !errors.Is(err, testCase.wantError) || result.Outcome != testCase.wantOutcome || result.BusinessEndpointDispatched != testCase.wantDispatched || result.RealExternalCallExecuted != testCase.wantReal {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if testCase.wantError == nil && result.OutcomeDigest == ([32]byte{}) {
				t.Fatal("classified provider result has no outcome digest")
			}
		})
	}
}
