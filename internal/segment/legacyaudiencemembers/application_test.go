package legacyaudiencemembers

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type packageReaderStub struct {
	exists bool
	err    error
	calls  []int64
}

func (stub *packageReaderStub) PackageExists(_ context.Context, packageID int64) (bool, error) {
	stub.calls = append(stub.calls, packageID)
	return stub.exists, stub.err
}

type memberRepositoryStub struct {
	page   MemberPage
	err    error
	inputs []ListInput
}

func (stub *memberRepositoryStub) ListMembers(_ context.Context, packageID int64, limit int, offset int64) (MemberPage, error) {
	stub.inputs = append(stub.inputs, ListInput{PackageID: packageID, Limit: limit, Offset: offset})
	return stub.page, stub.err
}

type identityReaderStub struct {
	items []TrustedExternalIdentity
	err   error
	calls [][]int64
}

func (stub *identityReaderStub) ListPrimaryExternalUserIDs(_ context.Context, customerIDs []int64) ([]TrustedExternalIdentity, error) {
	copyIDs := append([]int64(nil), customerIDs...)
	stub.calls = append(stub.calls, copyIDs)
	return append([]TrustedExternalIdentity(nil), stub.items...), stub.err
}

func TestServiceListMembersBuildsClosedSafeProjection(t *testing.T) {
	t.Parallel()
	latest := time.Date(2026, 8, 22, 4, 3, 2, 0, time.FixedZone("UTC+8", 8*60*60))
	tied := latest.Add(-time.Hour)
	packages := &packageReaderStub{exists: true}
	members := &memberRepositoryStub{page: MemberPage{
		Total: 7,
		Items: []MemberRecord{
			{CustomerID: 9, Nickname: "Alice", EnteredAt: latest},
			{CustomerID: 8, Nickname: "   ", EnteredAt: tied},
			{CustomerID: 7, Nickname: "Bob", EnteredAt: tied},
		},
	}}
	identities := &identityReaderStub{items: []TrustedExternalIdentity{
		{CustomerID: 9, ExternalUserID: "wm_primary_9"},
		{CustomerID: 7, ExternalUserID: "wm_primary_7"},
	}}
	service, err := NewService(packages, members, identities)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := service.ListMembers(context.Background(), ListInput{PackageID: 41, Limit: 3, Offset: 2})
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	if !response.OK || response.Total != 7 || response.Count != 3 || response.Limit != 3 || response.Offset != 2 || response.RealExternalCallExecuted {
		t.Fatalf("response metadata = %#v", response)
	}
	want := []MemberItem{
		{CustomerID: 9, Nickname: "Alice", ExternalUserID: "wm_primary_9", EnteredAt: latest.UTC()},
		{CustomerID: 8, Nickname: UnnamedCustomer, ExternalUserID: "", EnteredAt: tied.UTC()},
		{CustomerID: 7, Nickname: "Bob", ExternalUserID: "wm_primary_7", EnteredAt: tied.UTC()},
	}
	if !reflect.DeepEqual(response.Items, want) {
		t.Fatalf("items = %#v, want %#v", response.Items, want)
	}
	if !reflect.DeepEqual(packages.calls, []int64{41}) {
		t.Fatalf("existence calls = %v", packages.calls)
	}
	if !reflect.DeepEqual(members.inputs, []ListInput{{PackageID: 41, Limit: 3, Offset: 2}}) {
		t.Fatalf("repository inputs = %#v", members.inputs)
	}
	if !reflect.DeepEqual(identities.calls, [][]int64{{9, 8, 7}}) {
		t.Fatalf("identity calls = %#v", identities.calls)
	}
}

func TestServiceListMembersEmptyStateDoesNotResolveIdentities(t *testing.T) {
	t.Parallel()
	identities := &identityReaderStub{}
	service, err := NewService(
		&packageReaderStub{exists: true},
		&memberRepositoryStub{page: MemberPage{Items: []MemberRecord{}, Total: 0}},
		identities,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.ListMembers(context.Background(), ListInput{PackageID: 1, Limit: DefaultLimit})
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	if response.Items == nil || len(response.Items) != 0 || response.Total != 0 || response.Count != 0 {
		t.Fatalf("empty response = %#v", response)
	}
	if len(identities.calls) != 0 {
		t.Fatalf("identity reader called for empty page: %#v", identities.calls)
	}
}

func TestServiceListMembersNotFoundRequiresBothPackageFacts(t *testing.T) {
	t.Parallel()
	members := &memberRepositoryStub{}
	identities := &identityReaderStub{}
	service, _ := NewService(&packageReaderStub{exists: false}, members, identities)
	_, err := service.ListMembers(context.Background(), ListInput{PackageID: 90, Limit: 1})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if len(members.inputs) != 0 || len(identities.calls) != 0 {
		t.Fatal("not-found package must stop before member or identity reads")
	}
}

func TestServiceListMembersDependencyErrorsAreUnavailable(t *testing.T) {
	t.Parallel()
	dependencyErr := errors.New("database offline")
	tests := []struct {
		name       string
		packages   *packageReaderStub
		members    *memberRepositoryStub
		identities *identityReaderStub
	}{
		{"existence", &packageReaderStub{err: dependencyErr}, &memberRepositoryStub{}, &identityReaderStub{}},
		{"members", &packageReaderStub{exists: true}, &memberRepositoryStub{err: dependencyErr}, &identityReaderStub{}},
		{"identities", &packageReaderStub{exists: true}, &memberRepositoryStub{page: validMemberPage()}, &identityReaderStub{err: dependencyErr}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, _ := NewService(test.packages, test.members, test.identities)
			_, err := service.ListMembers(context.Background(), ListInput{PackageID: 1, Limit: 1})
			if !errors.Is(err, ErrUnavailable) || !errors.Is(err, dependencyErr) {
				t.Fatalf("error = %v, want joined unavailable dependency", err)
			}
		})
	}
}

func TestServiceListMembersRejectsInvalidInputAndUntrustedShapes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		input      ListInput
		page       MemberPage
		identities []TrustedExternalIdentity
		want       error
	}{
		{"non_positive_package", ListInput{PackageID: 0, Limit: 1}, MemberPage{}, nil, ErrNotFound},
		{"zero_limit", ListInput{PackageID: 1, Limit: 0}, MemberPage{}, nil, ErrInvalidInput},
		{"limit_too_large", ListInput{PackageID: 1, Limit: MaximumLimit + 1}, MemberPage{}, nil, ErrInvalidInput},
		{"negative_offset", ListInput{PackageID: 1, Limit: 1, Offset: -1}, MemberPage{}, nil, ErrInvalidInput},
		{"negative_total", ListInput{PackageID: 1, Limit: 1}, MemberPage{Items: []MemberRecord{}, Total: -1}, nil, ErrUnavailable},
		{"page_over_limit", ListInput{PackageID: 1, Limit: 1}, MemberPage{Items: []MemberRecord{{CustomerID: 2, EnteredAt: now}, {CustomerID: 1, EnteredAt: now}}, Total: 2}, nil, ErrUnavailable},
		{"unstable_time", ListInput{PackageID: 1, Limit: 2}, MemberPage{Items: []MemberRecord{{CustomerID: 2, EnteredAt: now}, {CustomerID: 1, EnteredAt: now.Add(time.Second)}}, Total: 2}, nil, ErrUnavailable},
		{"unstable_tie_id", ListInput{PackageID: 1, Limit: 2}, MemberPage{Items: []MemberRecord{{CustomerID: 1, EnteredAt: now}, {CustomerID: 2, EnteredAt: now}}, Total: 2}, nil, ErrUnavailable},
		{"duplicate_customer", ListInput{PackageID: 1, Limit: 2}, MemberPage{Items: []MemberRecord{{CustomerID: 1, EnteredAt: now}, {CustomerID: 1, EnteredAt: now.Add(-time.Second)}}, Total: 2}, nil, ErrUnavailable},
		{"zero_customer", ListInput{PackageID: 1, Limit: 1}, MemberPage{Items: []MemberRecord{{CustomerID: 0, EnteredAt: now}}, Total: 1}, nil, ErrUnavailable},
		{"zero_time", ListInput{PackageID: 1, Limit: 1}, MemberPage{Items: []MemberRecord{{CustomerID: 1}}, Total: 1}, nil, ErrUnavailable},
		{"identity_unknown_customer", ListInput{PackageID: 1, Limit: 1}, validMemberPage(), []TrustedExternalIdentity{{CustomerID: 999, ExternalUserID: "wm"}}, ErrUnavailable},
		{"identity_duplicate_customer", ListInput{PackageID: 1, Limit: 1}, validMemberPage(), []TrustedExternalIdentity{{CustomerID: 1, ExternalUserID: "wm1"}, {CustomerID: 1, ExternalUserID: "wm2"}}, ErrUnavailable},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, _ := NewService(
				&packageReaderStub{exists: true},
				&memberRepositoryStub{page: test.page},
				&identityReaderStub{items: test.identities},
			)
			_, err := service.ListMembers(context.Background(), test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewServiceRejectsNilDependencies(t *testing.T) {
	t.Parallel()
	var typedNil *identityReaderStub
	validPackages := &packageReaderStub{exists: true}
	validMembers := &memberRepositoryStub{}
	validIdentities := &identityReaderStub{}
	for _, test := range []struct {
		name       string
		packages   PackageExistenceReader
		members    MemberRepository
		identities TrustedIdentityReader
	}{
		{"packages", nil, validMembers, validIdentities},
		{"members", validPackages, nil, validIdentities},
		{"identities", validPackages, validMembers, nil},
		{"typed_nil", validPackages, validMembers, typedNil},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewService(test.packages, test.members, test.identities); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("NewService() error = %v", err)
			}
		})
	}
}

func validMemberPage() MemberPage {
	return MemberPage{
		Items: []MemberRecord{{CustomerID: 1, Nickname: "One", EnteredAt: time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)}},
		Total: 1,
	}
}
