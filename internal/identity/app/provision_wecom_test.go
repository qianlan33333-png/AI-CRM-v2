package app

import (
	"context"
	"errors"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type provisionStoreFake struct {
	current   ResolveRecord
	identity  NormalizedIdentity
	source    string
	bind      BindRecord
	bindCalls int
}

func (fake *provisionStoreFake) UpsertVerifiedWeCom(_ context.Context, identity NormalizedIdentity, source string) (int64, ResolveRecord, error) {
	fake.identity, fake.source = identity, source
	return 17, fake.current, nil
}

func (fake *provisionStoreFake) BindNormalized(context.Context, NormalizedIdentity, int64) (BindRecord, error) {
	fake.bindCalls++
	return fake.bind, nil
}

type provisionContactsFake struct {
	created  []contactport.CreateForIdentityCommand
	events   []contactport.ExternalEventCommand
	createID contactport.CustomerID
}

func (fake *provisionContactsFake) CreateForIdentity(_ context.Context, command contactport.CreateForIdentityCommand) (contactport.CustomerID, error) {
	fake.created = append(fake.created, command)
	return fake.createID, nil
}
func (*provisionContactsFake) MergeCustomers(context.Context, contactport.MergeCustomersCommand) error {
	return nil
}
func (fake *provisionContactsFake) AppendExternalEvent(_ context.Context, command contactport.ExternalEventCommand) (contactport.EventID, error) {
	fake.events = append(fake.events, command)
	return 1, nil
}

func TestVerifiedWeComProvisionCreatesBaseCustomerWithoutOwner(t *testing.T) {
	store := &provisionStoreFake{bind: BindRecord{Status: identityport.BindBound, IdentityID: 17}}
	contacts := &provisionContactsFake{createID: 41}
	events := &bindTestEvents{}
	service := NewVerifiedWeComProvisionService(&resolveTestUoW{}, store, contacts, events)

	result, err := service.ResolveOrCreate(context.Background(), verifiedSidebarRef())
	if err != nil || result.Status != identityport.ResolveFound || result.CustomerID != 41 {
		t.Fatalf("ResolveOrCreate() result=%+v err=%v", result, err)
	}
	if len(contacts.created) != 1 || contacts.created[0].OwnerStaffID != nil || contacts.created[0].ChannelID != nil {
		t.Fatalf("base customer command=%+v", contacts.created)
	}
	if store.bindCalls != 1 || len(contacts.events) != 1 || len(events.events) != 2 || contacts.events[0].EventType != eventport.EvCustomerAdded || events.events[1].Type != "identity.bound" {
		t.Fatalf("bind/events=%d/%+v/%+v", store.bindCalls, contacts.events, events.events)
	}
}

func TestVerifiedWeComProvisionReusesExistingCustomerWithoutCreatingRelationship(t *testing.T) {
	store := &provisionStoreFake{current: ResolveRecord{CustomerID: 52}}
	contacts := &provisionContactsFake{createID: 41}
	events := &bindTestEvents{}
	service := NewVerifiedWeComProvisionService(&resolveTestUoW{}, store, contacts, events)

	result, err := service.ResolveOrCreate(context.Background(), verifiedSidebarRef())
	if err != nil || result.CustomerID != 52 || len(contacts.created) != 0 || store.bindCalls != 0 || len(events.events) != 0 {
		t.Fatalf("result=%+v err=%v creates=%+v binds=%d events=%+v", result, err, contacts.created, store.bindCalls, events.events)
	}
}

func TestVerifiedWeComProvisionRejectsUntrustedOrConflictingIdentity(t *testing.T) {
	service := NewVerifiedWeComProvisionService(&resolveTestUoW{}, &provisionStoreFake{}, &provisionContactsFake{createID: 41}, &bindTestEvents{})
	untrusted := verifiedSidebarRef()
	untrusted.Assurance = identityport.AssuranceDeclared
	if _, err := service.ResolveOrCreate(context.Background(), untrusted); !errors.Is(err, ErrVerifiedWeComProvisionFailed) {
		t.Fatalf("untrusted error=%v", err)
	}
	service.store = &provisionStoreFake{current: ResolveRecord{Conflict: true}}
	if _, err := service.ResolveOrCreate(context.Background(), verifiedSidebarRef()); !errors.Is(err, ErrVerifiedWeComProvisionFailed) {
		t.Fatalf("conflict error=%v", err)
	}
}

func verifiedSidebarRef() identityport.IDRef {
	return identityport.IDRef{
		Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp-1", Value: "wm_external_41",
		Assurance: identityport.AssuranceVerified, Source: "sidebar.jssdk",
	}
}
