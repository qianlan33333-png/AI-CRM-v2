package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestCustomerMatcherServiceRequiresConsistentFoundHints(t *testing.T) {
	uow := &customerMatchUoW{}
	store := &customerMatchStoreStub{
		records: map[identityport.IDKind]ResolveRecord{
			identityport.KindPhone:               {CustomerID: 41},
			identityport.KindWeComExternalUserID: {CustomerID: 41},
		},
		unionIDs: []int64{41},
	}
	service := NewCustomerMatcherService(uow, store)
	matches, err := service.MatchCustomers(context.Background(), []identityport.CustomerMatchRequest{{
		CustomerID: 41,
		Refs: []identityport.IDRef{
			{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+8613800138000", Assurance: identityport.AssuranceVerified, Source: "survey.customer_answers"},
			{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp", Value: "ext-1", Assurance: identityport.AssuranceVerified, Source: "survey.customer_answers"},
		},
		LegacyUnionID: "union-1",
	}})
	if err != nil || len(matches) != 1 || !matches[0] || uow.calls != 1 || store.lookupCalls != 2 || store.unionCalls != 1 {
		t.Fatalf("matches/error/uow/lookups=%v/%v/%d/%d/%d", matches, err, uow.calls, store.lookupCalls, store.unionCalls)
	}
}

func TestCustomerMatcherServiceUsesOneUoWForMaximumSurveyBatch(t *testing.T) {
	uow := &customerMatchUoW{}
	store := &customerMatchStoreStub{
		records: map[identityport.IDKind]ResolveRecord{
			identityport.KindPhone:               {CustomerID: 41},
			identityport.KindWeComExternalUserID: {CustomerID: 41},
		},
		unionIDs: []int64{41},
	}
	requests := make([]identityport.CustomerMatchRequest, CustomerMatchMaximumBatch)
	for index := range requests {
		requests[index] = identityport.CustomerMatchRequest{
			CustomerID: 41,
			Refs: []identityport.IDRef{
				{Kind: identityport.KindPhone, Scope: "phone:e164", Value: fmt.Sprintf("+8613800%08d", index)},
				{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp", Value: fmt.Sprintf("ext-%d", index)},
			},
			LegacyUnionID: fmt.Sprintf("union-%d", index),
		}
	}
	matches, err := NewCustomerMatcherService(uow, store).MatchCustomers(context.Background(), requests)
	if err != nil || len(matches) != CustomerMatchMaximumBatch || uow.calls != 1 ||
		store.lookupCalls != CustomerMatchMaximumBatch*2 || store.unionCalls != CustomerMatchMaximumBatch {
		t.Fatalf("matches/error/uow/lookups=%d/%v/%d/%d/%d", len(matches), err, uow.calls, store.lookupCalls, store.unionCalls)
	}
	for index, matched := range matches {
		if !matched {
			t.Fatalf("match %d=false", index)
		}
	}
}

func TestCustomerMatcherServiceFailsWholeBatchClosedOnConflictOrDependencyError(t *testing.T) {
	tests := []struct {
		name  string
		store *customerMatchStoreStub
	}{
		{name: "different customers", store: &customerMatchStoreStub{records: map[identityport.IDKind]ResolveRecord{
			identityport.KindPhone: {CustomerID: 41}, identityport.KindWeComExternalUserID: {CustomerID: 42},
		}}},
		{name: "ambiguous", store: &customerMatchStoreStub{records: map[identityport.IDKind]ResolveRecord{
			identityport.KindPhone: {Conflict: true}, identityport.KindWeComExternalUserID: {},
		}}},
		{name: "union ambiguous", store: &customerMatchStoreStub{records: map[identityport.IDKind]ResolveRecord{
			identityport.KindPhone: {CustomerID: 41},
		}, unionIDs: []int64{41, 42}}},
		{name: "dependency", store: &customerMatchStoreStub{err: errors.New("identity unavailable")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uow := &customerMatchUoW{}
			matches, err := NewCustomerMatcherService(uow, test.store).MatchCustomers(context.Background(), []identityport.CustomerMatchRequest{
				{CustomerID: 41, Refs: []identityport.IDRef{{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+8613800138000"}, {Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp", Value: "ext-1"}}, LegacyUnionID: unionForTest(test.name)},
				{CustomerID: 41, Refs: []identityport.IDRef{{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+8613900139000"}}},
			})
			if matches != nil || !errors.Is(err, ErrCustomerIdentityMatchFailed) || uow.calls != 1 {
				t.Fatalf("matches/error/uow=%v/%v/%d", matches, err, uow.calls)
			}
		})
	}
}

func unionForTest(name string) string {
	if name == "union ambiguous" {
		return "union-1"
	}
	return ""
}

type customerMatchUoW struct{ calls int }

func (uow *customerMatchUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type customerMatchStoreStub struct {
	records     map[identityport.IDKind]ResolveRecord
	unionIDs    []int64
	err         error
	lookupCalls int
	unionCalls  int
}

func (store *customerMatchStoreStub) LookupNormalized(_ context.Context, identity NormalizedIdentity) (ResolveRecord, error) {
	store.lookupCalls++
	if store.err != nil {
		return ResolveRecord{}, store.err
	}
	return store.records[identity.Kind], nil
}

func (store *customerMatchStoreStub) LookupMessageArchiveUnionIDCustomers(context.Context, string) ([]int64, error) {
	store.unionCalls++
	if store.err != nil {
		return nil, store.err
	}
	return append([]int64(nil), store.unionIDs...), nil
}
