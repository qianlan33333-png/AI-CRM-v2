package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type resolveTestUoW struct {
	calls, callbacks int
	err              error
}

func (uow *resolveTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	uow.callbacks++
	return callback(ctx)
}

type resolveTestStore struct {
	record  ResolveRecord
	err     error
	calls   int
	lookups []NormalizedIdentity
}

func (store *resolveTestStore) LookupNormalized(_ context.Context, identity NormalizedIdentity) (ResolveRecord, error) {
	store.calls++
	store.lookups = append(store.lookups, identity)
	return store.record, store.err
}

func TestResolveServiceMapsNormalizedIdentityToExplicitStatuses(t *testing.T) {
	ref := identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 "}
	tests := []struct {
		name   string
		record ResolveRecord
		want   identityport.ResolveResult
	}{
		{name: "floating identity is not found", want: identityport.ResolveResult{Status: identityport.ResolveNotFound}},
		{name: "active binding is found", record: ResolveRecord{CustomerID: 42}, want: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(42)}},
		{name: "unavailable trusted binding is conflict", record: ResolveRecord{Conflict: true, CustomerID: 42}, want: identityport.ResolveResult{Status: identityport.ResolveConflict}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &resolveTestUoW{}, &resolveTestStore{record: testCase.record}
			result, err := NewResolveService(uow, store).Resolve(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			if result != testCase.want {
				t.Fatalf("Resolve() result=%+v, want %+v", result, testCase.want)
			}
			if uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("calls: uow=%d callbacks=%d store=%d", uow.calls, uow.callbacks, store.calls)
			}
			wantLookup := NormalizedIdentity{Kind: identityport.KindPhone, Scope: "phone:e164", NormalizedValue: "+8613800138000", NormalizerVersion: NormalizerVersion}
			if !reflect.DeepEqual(store.lookups, []NormalizedIdentity{wantLookup}) {
				t.Fatalf("lookup=%+v, want %+v", store.lookups, wantLookup)
			}
		})
	}
}

func TestResolveServiceRejectsInvalidIdentityBeforeLookup(t *testing.T) {
	uow, store := &resolveTestUoW{}, &resolveTestStore{}
	_, err := NewResolveService(uow, store).Resolve(context.Background(), identityport.IDRef{
		Kind: identityport.KindPhone, Scope: "phone:e164", Value: "13800138000",
	})
	if !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("Resolve() error=%v, want ErrInvalidIdentity", err)
	}
	if uow.calls != 0 || store.calls != 0 {
		t.Fatalf("invalid Resolve called dependencies: uow=%d store=%d", uow.calls, store.calls)
	}
}

func TestResolveServiceFailsClosedForDependencyAndMalformedRecords(t *testing.T) {
	ref := identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:test", Value: "lookup"}
	sentinel := errors.New("store unavailable")
	tests := []struct {
		name    string
		service *ResolveService
		want    error
	}{
		{name: "nil service", want: ErrIdentityResolveFailed},
		{name: "missing unit of work", service: &ResolveService{store: &resolveTestStore{}}, want: ErrIdentityResolveFailed},
		{name: "missing store", service: &ResolveService{uow: &resolveTestUoW{}}, want: ErrIdentityResolveFailed},
		{name: "unit of work error", service: NewResolveService(&resolveTestUoW{err: sentinel}, &resolveTestStore{}), want: sentinel},
		{name: "store error", service: NewResolveService(&resolveTestUoW{}, &resolveTestStore{err: sentinel}), want: sentinel},
		{name: "negative customer id", service: NewResolveService(&resolveTestUoW{}, &resolveTestStore{record: ResolveRecord{CustomerID: -1}}), want: ErrIdentityResolveFailed},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.service.Resolve(context.Background(), ref)
			if !errors.Is(err, ErrIdentityResolveFailed) || !errors.Is(err, testCase.want) {
				t.Fatalf("Resolve() error=%v, want identity failure containing %v", err, testCase.want)
			}
		})
	}
}
