package app

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestChannelAcquisitionEntrantKnownIdentityUsesHistoricalExactMatch(t *testing.T) {
	match := entrantMatch(41, 7, contactport.AcquisitionAssetQRCode, 3)
	service, correlation, identities, receipts := entrantServiceFixture(t)
	correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: match}
	identities.result = identityport.AcquisitionEntrantIdentityResolution{Status: identityport.AcquisitionEntrantIdentityFound, CustomerID: 22}

	result, err := service.Process(context.Background(), entrantInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.IdentityPendingRequired || result.Receipt.Status != contactport.ChannelAcquisitionEntrantAttributed || result.Receipt.AssetVersion != 3 || result.Receipt.CustomerID != 22 || result.Receipt.CustomerEventID < 1 {
		t.Fatalf("result = %#v", result)
	}
	if correlation.calls != 1 || correlation.corpID != "corp-a" || correlation.state != "ch02_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" || !correlation.occurredAt.Equal(entrantInput().Fact.OccurredAt) {
		t.Fatalf("correlation = %#v", correlation)
	}
	if identities.calls != 1 || identities.ref.Kind != identityport.KindWeComExternalUserID || identities.ref.Scope != "wecom-corp:corp-a" || identities.ref.Value != "external-1" {
		t.Fatalf("identity = %#v", identities)
	}
	if receipts.calls != 1 || receipts.command.Status != contactport.ChannelAcquisitionEntrantAttributed || receipts.command.Match != match || receipts.command.CustomerID != 22 || receipts.command.WeComUserID != "staff-1" {
		t.Fatalf("receipt command = %#v", receipts.command)
	}
}

func TestChannelAcquisitionEntrantReplaysReconciledReceipt(t *testing.T) {
	match := entrantMatch(42, 8, contactport.AcquisitionAssetLink, 2)
	service, correlation, identities, receipts := entrantServiceFixture(t)
	correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: match}
	identities.result = identityport.AcquisitionEntrantIdentityResolution{Status: identityport.AcquisitionEntrantIdentityFound, CustomerID: 22}
	receipts.resultStatus = contactport.ChannelAcquisitionEntrantReconciled

	first, err := service.Process(context.Background(), entrantInput())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Process(context.Background(), entrantInput())
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt != second.Receipt || first.Receipt.Status != contactport.ChannelAcquisitionEntrantReconciled || receipts.calls != 2 {
		t.Fatalf("replays = %#v / %#v, calls=%d", first, second, receipts.calls)
	}
}

func TestChannelAcquisitionEntrantNeverGuessesZeroOrMultipleAssetMatches(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		cardinality contactport.AcquisitionAssetCorrelationCardinality
		want        contactport.ChannelAcquisitionEntrantStatus
	}{
		{name: "zero", cardinality: contactport.AcquisitionAssetCorrelationZero, want: contactport.ChannelAcquisitionEntrantUnmatchedAsset},
		{name: "multiple", cardinality: contactport.AcquisitionAssetCorrelationMultiple, want: contactport.ChannelAcquisitionEntrantAmbiguousAsset},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service, correlation, identities, receipts := entrantServiceFixture(t)
			correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: testCase.cardinality}
			result, err := service.Process(context.Background(), entrantInput())
			if err != nil {
				t.Fatal(err)
			}
			if result.Receipt.Status != testCase.want || identities.calls != 0 || receipts.command.Match != (contactport.AcquisitionAssetCorrelationMatch{}) || receipts.command.CustomerID != 0 {
				t.Fatalf("result=%#v identity=%#v receipt=%#v", result, identities, receipts.command)
			}
		})
	}
}

func TestChannelAcquisitionEntrantPendingAndConflictNeverCreateCustomer(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		identity    identityport.AcquisitionEntrantIdentityStatus
		want        contactport.ChannelAcquisitionEntrantStatus
		wantPending bool
	}{
		{name: "pending", identity: identityport.AcquisitionEntrantIdentityNotFound, want: contactport.ChannelAcquisitionEntrantPendingIdentity, wantPending: true},
		{name: "conflict", identity: identityport.AcquisitionEntrantIdentityConflict, want: contactport.ChannelAcquisitionEntrantConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service, correlation, identities, receipts := entrantServiceFixture(t)
			correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: entrantMatch(41, 7, contactport.AcquisitionAssetQRCode, 1)}
			identities.result = identityport.AcquisitionEntrantIdentityResolution{Status: testCase.identity}
			result, err := service.Process(context.Background(), entrantInput())
			if err != nil {
				t.Fatal(err)
			}
			if result.Receipt.Status != testCase.want || result.IdentityPendingRequired != testCase.wantPending || receipts.command.CustomerID != 0 || result.Receipt.CustomerID != 0 || result.Receipt.CustomerEventID != 0 {
				t.Fatalf("result=%#v receipt=%#v", result, receipts.command)
			}
		})
	}
}

func TestChannelAcquisitionEntrantAssigneeMismatchIsContactFailure(t *testing.T) {
	service, correlation, identities, receipts := entrantServiceFixture(t)
	correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: entrantMatch(41, 7, contactport.AcquisitionAssetQRCode, 1)}
	identities.result = identityport.AcquisitionEntrantIdentityResolution{Status: identityport.AcquisitionEntrantIdentityFound, CustomerID: 22}
	receipts.err = errors.New("callback user is not a frozen assignee")

	if _, err := service.Process(context.Background(), entrantInput()); !errors.Is(err, ErrChannelAcquisitionEntrantFailed) {
		t.Fatalf("error = %v", err)
	}
	if receipts.calls != 1 || receipts.command.WeComUserID != "staff-1" {
		t.Fatalf("receipt command = %#v", receipts.command)
	}
}

func TestChannelAcquisitionEntrantIgnoredDoesNotResolveAssetOrIdentity(t *testing.T) {
	service, correlation, identities, receipts := entrantServiceFixture(t)
	input := entrantInput()
	input.Fact.ChangeType = "del_external_contact"
	input.Fact.UserID = ""
	input.Fact.State = ""
	result, err := service.Process(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Status != contactport.ChannelAcquisitionEntrantIgnored || correlation.calls != 0 || identities.calls != 0 || receipts.command.Status != contactport.ChannelAcquisitionEntrantIgnored {
		t.Fatalf("result=%#v correlation=%#v identity=%#v receipt=%#v", result, correlation, identities, receipts.command)
	}
}

func TestChannelAcquisitionEntrantStatusSetIsClosed(t *testing.T) {
	for _, status := range []contactport.ChannelAcquisitionEntrantStatus{
		contactport.ChannelAcquisitionEntrantCorrelated,
		contactport.ChannelAcquisitionEntrantAttributed,
		contactport.ChannelAcquisitionEntrantPendingIdentity,
		contactport.ChannelAcquisitionEntrantUnmatchedAsset,
		contactport.ChannelAcquisitionEntrantAmbiguousAsset,
		contactport.ChannelAcquisitionEntrantConflict,
		contactport.ChannelAcquisitionEntrantIgnored,
		contactport.ChannelAcquisitionEntrantReconciled,
	} {
		if !status.Valid() {
			t.Fatalf("status %q must be valid", status)
		}
	}
	if contactport.ChannelAcquisitionEntrantStatus("latest_match").Valid() {
		t.Fatal("invented status must not be valid")
	}
}

type entrantTestUoW struct{ calls int }

func (uow *entrantTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type entrantCorrelation struct {
	calls         int
	corpID, state string
	occurredAt    time.Time
	result        contactport.AcquisitionAssetCorrelationResolution
	err           error
}

func (resolver *entrantCorrelation) ResolveAcquisitionAssetCorrelation(_ context.Context, corpID, state string, occurredAt time.Time) (contactport.AcquisitionAssetCorrelationResolution, error) {
	resolver.calls++
	resolver.corpID, resolver.state, resolver.occurredAt = corpID, state, occurredAt
	return resolver.result, resolver.err
}

type entrantIdentityResolver struct {
	calls  int
	ref    identityport.IDRef
	result identityport.AcquisitionEntrantIdentityResolution
	err    error
}

func (resolver *entrantIdentityResolver) ResolveAcquisitionEntrantIdentity(_ context.Context, ref identityport.IDRef) (identityport.AcquisitionEntrantIdentityResolution, error) {
	resolver.calls++
	resolver.ref = ref
	return resolver.result, resolver.err
}

type entrantReceipts struct {
	calls        int
	command      contactport.ChannelAcquisitionEntrantCommand
	resultStatus contactport.ChannelAcquisitionEntrantStatus
	err          error
}

func (receipts *entrantReceipts) RecordChannelAcquisitionEntrant(_ context.Context, command contactport.ChannelAcquisitionEntrantCommand) (contactport.ChannelAcquisitionEntrantReceipt, error) {
	receipts.calls++
	receipts.command = command
	if receipts.err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, receipts.err
	}
	status := receipts.resultStatus
	if status == "" {
		status = command.Status
	}
	receipt := contactport.ChannelAcquisitionEntrantReceipt{ID: 91, InboxID: command.InboxID, Status: status, OccurredAt: command.OccurredAt}
	if command.Match != (contactport.AcquisitionAssetCorrelationMatch{}) {
		receipt.EffectID, receipt.ChannelID, receipt.Kind, receipt.AssetVersion = command.Match.EffectID, command.Match.ChannelID, command.Match.Kind, command.Match.AssetVersion
	}
	if command.Status == contactport.ChannelAcquisitionEntrantAttributed {
		receipt.CustomerID, receipt.CustomerEventID = command.CustomerID, 16
	}
	return receipt, nil
}

func entrantServiceFixture(t *testing.T) (*ChannelAcquisitionEntrantService, *entrantCorrelation, *entrantIdentityResolver, *entrantReceipts) {
	t.Helper()
	correlation := &entrantCorrelation{}
	identities := &entrantIdentityResolver{}
	receipts := &entrantReceipts{}
	service, err := NewChannelAcquisitionEntrantService(&entrantTestUoW{}, correlation, identities, receipts)
	if err != nil {
		t.Fatal(err)
	}
	return service, correlation, identities, receipts
}

func entrantInput() ChannelAcquisitionEntrantInput {
	return ChannelAcquisitionEntrantInput{InboxID: 71, SourceKey: "sha256:" + repeatedHex('b'), Fact: ExternalContactCallbackFact{
		CorpID: "corp-a", ChangeType: addExternalContact, ExternalUserID: "external-1", UserID: "staff-1",
		State: "ch02_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", OccurredAt: time.Unix(1700000000, 0).UTC(),
	}}
}

func entrantMatch(effectID, channelID int64, kind contactport.AcquisitionAssetKind, version int64) contactport.AcquisitionAssetCorrelationMatch {
	return contactport.AcquisitionAssetCorrelationMatch{EffectID: "eer_" + strconv.FormatInt(effectID, 10), ChannelID: channelID, Kind: kind, AssetVersion: version}
}
