package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
)

type audienceMembersTransactionMarker struct{}

type audienceMembersUoWStub struct {
	calls    int
	attempts int
}

func (stub *audienceMembersUoWStub) Within(ctx context.Context, callback func(context.Context) error) error {
	stub.calls++
	if active, _ := ctx.Value(audienceMembersTransactionMarker{}).(bool); active {
		return platformport.ErrNestedTransaction
	}
	attempts := stub.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err := callback(context.WithValue(ctx, audienceMembersTransactionMarker{}, true)); err != nil {
			return err
		}
	}
	return nil
}

type audienceMembersPackageReaderStub struct {
	transactionMarker bool
}

func (stub *audienceMembersPackageReaderStub) PackageExists(ctx context.Context, _ int64) (bool, error) {
	stub.transactionMarker, _ = ctx.Value(audienceMembersTransactionMarker{}).(bool)
	return true, nil
}

type audienceMembersMemberReaderStub struct {
	transactionMarker bool
}

func (stub *audienceMembersMemberReaderStub) ListMembers(
	ctx context.Context,
	_ int64,
	_ int,
	_ int64,
) (legacyaudiencemembers.MemberPage, error) {
	stub.transactionMarker, _ = ctx.Value(audienceMembersTransactionMarker{}).(bool)
	return legacyaudiencemembers.MemberPage{Total: 1, Items: []legacyaudiencemembers.MemberRecord{{
		CustomerID: 17, Nickname: "member", EnteredAt: time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC),
	}}}, nil
}

type audienceMembersIdentityReaderStub struct {
	transactionMarker bool
	gotIDs            []contactport.CustomerID
	items             []identityport.TrustedWeComExternalIdentity
	calls             int
}

func (stub *audienceMembersIdentityReaderStub) ListPrimaryWeComExternalUserIDs(
	ctx context.Context,
	customerIDs []contactport.CustomerID,
) ([]identityport.TrustedWeComExternalIdentity, error) {
	stub.calls++
	stub.transactionMarker, _ = ctx.Value(audienceMembersTransactionMarker{}).(bool)
	stub.gotIDs = append([]contactport.CustomerID(nil), customerIDs...)
	return append([]identityport.TrustedWeComExternalIdentity(nil), stub.items...), nil
}

func TestAIAudienceMembersApplicationUsesOneOuterTransactionForBothOwners(t *testing.T) {
	uow := &audienceMembersUoWStub{}
	packages := &audienceMembersPackageReaderStub{}
	members := &audienceMembersMemberReaderStub{}
	identities := &audienceMembersIdentityReaderStub{items: []identityport.TrustedWeComExternalIdentity{{
		CustomerID: 17, ExternalUserID: "wm_trusted",
	}}}
	service, err := legacyaudiencemembers.NewService(
		packages,
		members,
		legacyAIAudienceMembersIdentityReader{reader: identities},
	)
	if err != nil {
		t.Fatal(err)
	}
	application := legacyAIAudienceMembersApplication{uow: uow, application: service}

	response, err := application.ListMembers(context.Background(), legacyaudiencemembers.ListInput{PackageID: 7, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || !packages.transactionMarker || !members.transactionMarker || !identities.transactionMarker {
		t.Fatalf("uow=%d package-tx=%t member-tx=%t identity-tx=%t", uow.calls, packages.transactionMarker, members.transactionMarker, identities.transactionMarker)
	}
	want := []legacyaudiencemembers.MemberItem{{
		CustomerID: 17, Nickname: "member", ExternalUserID: "wm_trusted", EnteredAt: time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC),
	}}
	if !reflect.DeepEqual(response.Items, want) || !reflect.DeepEqual(identities.gotIDs, []contactport.CustomerID{17}) {
		t.Fatalf("response=%#v identity-ids=%v want=%#v", response, identities.gotIDs, want)
	}
}

func TestAIAudienceMembersApplicationRejectsNestedTransaction(t *testing.T) {
	uow := &audienceMembersUoWStub{}
	application := legacyAIAudienceMembersApplication{uow: uow, application: &audienceMembersApplicationStub{}}
	ctx := context.WithValue(context.Background(), audienceMembersTransactionMarker{}, true)

	_, err := application.ListMembers(ctx, legacyaudiencemembers.ListInput{PackageID: 7, Limit: 1})
	if !errors.Is(err, platformport.ErrNestedTransaction) || !errors.Is(err, legacyaudiencemembers.ErrUnavailable) {
		t.Fatalf("error=%v, want nested transaction and unavailable", err)
	}
}

func TestAIAudienceMembersApplicationRetryOverwritesAttemptResult(t *testing.T) {
	uow := &audienceMembersUoWStub{attempts: 2}
	inner := &audienceMembersApplicationStub{response: legacyaudiencemembers.ListResponse{
		OK: true, Items: []legacyaudiencemembers.MemberItem{{CustomerID: 17}}, Count: 1,
	}}
	application := legacyAIAudienceMembersApplication{uow: uow, application: inner}

	response, err := application.ListMembers(context.Background(), legacyaudiencemembers.ListInput{PackageID: 7, Limit: 1})
	if err != nil || inner.calls != 2 || len(response.Items) != 1 || response.Items[0].CustomerID != 17 {
		t.Fatalf("response=%#v err=%v calls=%d", response, err, inner.calls)
	}
}

func TestAIAudienceMembersIdentityReaderEnforcesMaximumBeforeStore(t *testing.T) {
	reader := &audienceMembersIdentityReaderStub{}
	adapter := legacyAIAudienceMembersIdentityReader{reader: reader}
	ids := make([]int64, identityport.MaximumTrustedWeComIdentityCustomerIDs+1)
	for index := range ids {
		ids[index] = int64(index + 1)
	}

	_, err := adapter.ListPrimaryExternalUserIDs(context.Background(), ids)
	if err == nil || reader.calls != 0 {
		t.Fatalf("error=%v reader calls=%d", err, reader.calls)
	}
}

type audienceMembersApplicationStub struct {
	response legacyaudiencemembers.ListResponse
	err      error
	calls    int
}

func (stub *audienceMembersApplicationStub) ListMembers(
	_ context.Context,
	_ legacyaudiencemembers.ListInput,
) (legacyaudiencemembers.ListResponse, error) {
	stub.calls++
	return stub.response, stub.err
}

var _ platformport.UnitOfWork = (*audienceMembersUoWStub)(nil)
var _ identityport.TrustedWeComIdentityReader = (*audienceMembersIdentityReaderStub)(nil)
var _ legacyaudiencemembers.Application = (*audienceMembersApplicationStub)(nil)
