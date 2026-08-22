package app

import (
	"context"
	"errors"
	"testing"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestCustomerMatcherServiceRequiresConsistentFoundHints(t *testing.T) {
	resolver := &customerMatchResolverStub{results: map[identityport.IDKind]identityport.ResolveResult{
		identityport.KindPhone:               {Status: identityport.ResolveFound, CustomerID: 41},
		identityport.KindWeComExternalUserID: {Status: identityport.ResolveFound, CustomerID: 41},
	}}
	service := NewCustomerMatcherService(resolver, &customerMatchUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveNotFound}})
	matched, err := service.MatchesCustomer(context.Background(), identityport.CustomerMatchRequest{
		CustomerID: 41,
		Refs: []identityport.IDRef{
			{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+8613800138000", Assurance: identityport.AssuranceVerified, Source: "survey.customer_answers"},
			{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp", Value: "ext-1", Assurance: identityport.AssuranceVerified, Source: "survey.customer_answers"},
		},
		LegacyUnionID: "union-1",
	})
	if err != nil || !matched || len(resolver.refs) != 2 {
		t.Fatalf("matched/err/refs=%t/%v/%v", matched, err, resolver.refs)
	}
}

func TestCustomerMatcherServiceFailsClosedOnConflictOrDependencyError(t *testing.T) {
	tests := []struct {
		name     string
		resolver *customerMatchResolverStub
	}{
		{name: "different customers", resolver: &customerMatchResolverStub{results: map[identityport.IDKind]identityport.ResolveResult{
			identityport.KindPhone: {Status: identityport.ResolveFound, CustomerID: 41}, identityport.KindWeComExternalUserID: {Status: identityport.ResolveFound, CustomerID: 42},
		}}},
		{name: "ambiguous", resolver: &customerMatchResolverStub{results: map[identityport.IDKind]identityport.ResolveResult{
			identityport.KindPhone: {Status: identityport.ResolveConflict}, identityport.KindWeComExternalUserID: {Status: identityport.ResolveNotFound},
		}}},
		{name: "dependency", resolver: &customerMatchResolverStub{err: errors.New("identity unavailable")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewCustomerMatcherService(test.resolver, nil)
			matched, err := service.MatchesCustomer(context.Background(), identityport.CustomerMatchRequest{CustomerID: 41, Refs: []identityport.IDRef{
				{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+8613800138000"},
				{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp", Value: "ext-1"},
			}})
			if matched || !errors.Is(err, ErrCustomerIdentityMatchFailed) {
				t.Fatalf("matched/err=%t/%v", matched, err)
			}
		})
	}
}

type customerMatchResolverStub struct {
	results map[identityport.IDKind]identityport.ResolveResult
	err     error
	refs    []identityport.IDRef
}

func (stub *customerMatchResolverStub) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	stub.refs = append(stub.refs, ref)
	if stub.err != nil {
		return identityport.ResolveResult{}, stub.err
	}
	return stub.results[ref.Kind], nil
}

type customerMatchUnionStub struct {
	result identityport.ResolveResult
	err    error
}

func (stub *customerMatchUnionStub) ResolveUnionID(context.Context, string) (identityport.ResolveResult, error) {
	return stub.result, stub.err
}
