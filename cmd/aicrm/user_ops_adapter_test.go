package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

func TestUserOpsPhoneFilterResolvesFormattedInputOnlyInsideIdentity(t *testing.T) {
	store := &userOpsIdentityLookupStub{record: identityapp.ResolveRecord{CustomerID: 71}}
	reader := userOpsDirectoryReader{
		customers:  contactapp.NewCustomerListService(nil, nil),
		identities: identityapp.NewResolveService(nil, store),
	}

	input, err := reader.customerListInput(context.Background(), useropsport.DirectoryQuery{
		PhoneExact: " +86 (138) 0013-8000 ",
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if input.CustomerID == nil || *input.CustomerID != 71 || input.MatchNone {
		t.Fatalf("customer input = %#v, want resolved canonical customer only", input)
	}
	wantLookup := identityapp.NormalizedIdentity{
		Kind:              identityport.KindPhone,
		Scope:             "phone:e164",
		NormalizedValue:   "+8613800138000",
		NormalizerVersion: identityapp.NormalizerVersion,
	}
	if !reflect.DeepEqual(store.lookups, []identityapp.NormalizedIdentity{wantLookup}) {
		t.Fatalf("identity lookup = %#v, want %#v", store.lookups, []identityapp.NormalizedIdentity{wantLookup})
	}
}

func TestUserOpsPhoneFilterFailsClosedOnIdentityConflict(t *testing.T) {
	store := &userOpsIdentityLookupStub{record: identityapp.ResolveRecord{Conflict: true}}
	reader := userOpsDirectoryReader{
		customers:  contactapp.NewCustomerListService(nil, nil),
		identities: identityapp.NewResolveService(nil, store),
	}

	_, err := reader.customerListInput(context.Background(), useropsport.DirectoryQuery{PhoneExact: "+8613800138000"}, 10)
	if !errors.Is(err, useropsport.ErrConflict) {
		t.Fatalf("customerListInput() error = %v, want identity conflict", err)
	}
	if len(store.lookups) != 1 {
		t.Fatalf("identity lookups = %d, want 1", len(store.lookups))
	}
}

type userOpsIdentityLookupStub struct {
	record  identityapp.ResolveRecord
	lookups []identityapp.NormalizedIdentity
}

func (store *userOpsIdentityLookupStub) LookupNormalized(_ context.Context, identity identityapp.NormalizedIdentity) (identityapp.ResolveRecord, error) {
	store.lookups = append(store.lookups, identity)
	return store.record, nil
}
