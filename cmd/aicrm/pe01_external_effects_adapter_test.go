package main

import (
	"context"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestPE01ProviderAdapterPreservesExternalCallEvidence(t *testing.T) {
	receipt := [32]byte{1}
	adapter := pe01ProviderAdapter{execute: func(context.Context) (orderport.ProviderResult, error) {
		return orderport.ProviderResult{
			Completion: orderport.ProviderOutcomeUnknown, ReceiptDigest: receipt,
			BusinessCallDispatched: true, RealExternalCallExecuted: true,
		}, nil
	}}
	result, err := adapter.Execute(context.Background(), eer.EffectEnvelope{}, eer.Attempt{})
	if err != nil || result.Completion != eer.CompletionOutcomeUnknown || result.ReceiptDigest != eerDigest(receipt) || !result.BusinessCallDispatched || !result.RealExternalCallExecuted {
		t.Fatalf("adapter result=%+v err=%v", result, err)
	}
}
