package main

import (
	"context"
	"reflect"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
)

type audienceMembersIdentityContextKey struct{}

type audienceMembersIdentityUoWStub struct {
	called   int
	attempts int
}

func (stub *audienceMembersIdentityUoWStub) Within(ctx context.Context, callback func(context.Context) error) error {
	stub.called++
	attempts := stub.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err := callback(context.WithValue(ctx, audienceMembersIdentityContextKey{}, true)); err != nil {
			return err
		}
	}
	return nil
}

type audienceMembersIdentityReaderStub struct {
	gotContext bool
	gotIDs     []contactport.CustomerID
	items      []identityport.TrustedWeComExternalIdentity
	calls      int
}

func (stub *audienceMembersIdentityReaderStub) ListPrimaryWeComExternalUserIDs(
	ctx context.Context,
	customerIDs []contactport.CustomerID,
) ([]identityport.TrustedWeComExternalIdentity, error) {
	stub.calls++
	stub.gotContext, _ = ctx.Value(audienceMembersIdentityContextKey{}).(bool)
	stub.gotIDs = append([]contactport.CustomerID(nil), customerIDs...)
	return append([]identityport.TrustedWeComExternalIdentity(nil), stub.items...), nil
}

func TestAIAudienceMembersIdentityReaderEnforcesMaximumBeforeUoWAndStore(t *testing.T) {
	uow := &audienceMembersIdentityUoWStub{}
	reader := &audienceMembersIdentityReaderStub{}
	adapter := legacyAIAudienceMembersIdentityReader{uow: uow, reader: reader}
	ids := make([]int64, identityport.MaximumTrustedWeComIdentityCustomerIDs+1)
	for index := range ids {
		ids[index] = int64(index + 1)
	}

	_, err := adapter.ListPrimaryExternalUserIDs(context.Background(), ids)
	if err == nil || uow.called != 0 || reader.calls != 0 {
		t.Fatalf("error=%v uow calls=%d reader calls=%d", err, uow.called, reader.calls)
	}
	items, err := adapter.ListPrimaryExternalUserIDs(context.Background(), ids[:identityport.MaximumTrustedWeComIdentityCustomerIDs])
	if err != nil || len(items) != 0 || uow.called != 1 || reader.calls != 1 {
		t.Fatalf("maximum items=%v error=%v uow calls=%d reader calls=%d", items, err, uow.called, reader.calls)
	}
}

func TestAIAudienceMembersIdentityReaderRetryOverwritesAttemptResult(t *testing.T) {
	uow := &audienceMembersIdentityUoWStub{attempts: 2}
	reader := &audienceMembersIdentityReaderStub{items: []identityport.TrustedWeComExternalIdentity{{
		CustomerID: 17, ExternalUserID: "wm_trusted",
	}}}
	adapter := legacyAIAudienceMembersIdentityReader{uow: uow, reader: reader}

	items, err := adapter.ListPrimaryExternalUserIDs(context.Background(), []int64{17})
	want := []legacyaudiencemembers.TrustedExternalIdentity{{CustomerID: 17, ExternalUserID: "wm_trusted"}}
	if err != nil || !reflect.DeepEqual(items, want) || reader.calls != 2 {
		t.Fatalf("items=%#v err=%v calls=%d want=%#v", items, err, reader.calls, want)
	}
}

func TestAIAudienceMembersIdentityReaderUsesOuterTransactionContextAndMapsIdentityDTO(t *testing.T) {
	uow := &audienceMembersIdentityUoWStub{}
	reader := &audienceMembersIdentityReaderStub{items: []identityport.TrustedWeComExternalIdentity{{
		CustomerID: 17, ExternalUserID: "wm_trusted",
	}}}
	adapter := legacyAIAudienceMembersIdentityReader{uow: uow, reader: reader}

	items, err := adapter.ListPrimaryExternalUserIDs(context.Background(), []int64{17})
	if err != nil {
		t.Fatal(err)
	}
	if uow.called != 1 || !reader.gotContext || !reflect.DeepEqual(reader.gotIDs, []contactport.CustomerID{17}) {
		t.Fatalf("uow calls=%d tx-context=%t ids=%v", uow.called, reader.gotContext, reader.gotIDs)
	}
	if want := []legacyaudiencemembers.TrustedExternalIdentity{{CustomerID: 17, ExternalUserID: "wm_trusted"}}; !reflect.DeepEqual(items, want) {
		t.Fatalf("items=%#v want=%#v", items, want)
	}
}

var _ platformport.UnitOfWork = (*audienceMembersIdentityUoWStub)(nil)
var _ identityport.TrustedWeComIdentityReader = (*audienceMembersIdentityReaderStub)(nil)
