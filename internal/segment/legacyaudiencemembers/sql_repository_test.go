package legacyaudiencemembers

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

type fakeAudienceMemberQueries struct {
	exists        bool
	existsErr     error
	existsInputs  []int64
	memberRows    []segmentdb.ListLegacyAudienceMembersRow
	membersErr    error
	membersInputs []segmentdb.ListLegacyAudienceMembersParams
}

func (queries *fakeAudienceMemberQueries) LegacyAudiencePackageExists(_ context.Context, packageID int64) (bool, error) {
	queries.existsInputs = append(queries.existsInputs, packageID)
	return queries.exists, queries.existsErr
}

func (queries *fakeAudienceMemberQueries) ListLegacyAudienceMembers(
	_ context.Context,
	input segmentdb.ListLegacyAudienceMembersParams,
) ([]segmentdb.ListLegacyAudienceMembersRow, error) {
	queries.membersInputs = append(queries.membersInputs, input)
	return append([]segmentdb.ListLegacyAudienceMembersRow(nil), queries.memberRows...), queries.membersErr
}

func TestSQLRepositoryMapsClosedMemberSnapshot(t *testing.T) {
	t.Parallel()
	latest := time.Date(2026, 8, 22, 3, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	older := latest.Add(-time.Minute)
	queries := &fakeAudienceMemberQueries{
		exists: true,
		memberRows: []segmentdb.ListLegacyAudienceMembersRow{
			{Total: 5, CustomerID: pgtype.Int8{Int64: 9, Valid: true}, Name: pgtype.Text{String: "Nine", Valid: true}, ComputedAt: pgtype.Timestamptz{Time: latest, Valid: true}},
			{Total: 5, CustomerID: pgtype.Int8{Int64: 7, Valid: true}, Name: pgtype.Text{String: "", Valid: true}, ComputedAt: pgtype.Timestamptz{Time: older, Valid: true}},
		},
	}
	repository, err := newSQLRepository(queries)
	if err != nil {
		t.Fatal(err)
	}
	exists, err := repository.PackageExists(context.Background(), 22)
	if err != nil || !exists {
		t.Fatalf("PackageExists() = %v, %v", exists, err)
	}
	page, err := repository.ListMembers(context.Background(), 22, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := MemberPage{Total: 5, Items: []MemberRecord{
		{CustomerID: 9, Nickname: "Nine", EnteredAt: latest.UTC()},
		{CustomerID: 7, Nickname: "", EnteredAt: older.UTC()},
	}}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("page = %#v, want %#v", page, want)
	}
	if !reflect.DeepEqual(queries.existsInputs, []int64{22}) {
		t.Fatalf("existence inputs = %#v", queries.existsInputs)
	}
	wantInput := []segmentdb.ListLegacyAudienceMembersParams{{PackageID: 22, RowLimit: 2, RowOffset: 1}}
	if !reflect.DeepEqual(queries.membersInputs, wantInput) {
		t.Fatalf("member inputs = %#v, want %#v", queries.membersInputs, wantInput)
	}
}

func TestSQLRepositoryEmptyPageIsNonNil(t *testing.T) {
	t.Parallel()
	repository, _ := newSQLRepository(&fakeAudienceMemberQueries{memberRows: []segmentdb.ListLegacyAudienceMembersRow{{Total: 0}}})
	page, err := repository.ListMembers(context.Background(), 1, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil || len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("empty page = %#v", page)
	}
}

func TestSQLRepositoryFailsClosed(t *testing.T) {
	t.Parallel()
	dependencyErr := errors.New("database failure")
	tests := []struct {
		name    string
		queries *fakeAudienceMemberQueries
		call    func(*SQLRepository) error
	}{
		{"existence", &fakeAudienceMemberQueries{existsErr: dependencyErr}, func(repository *SQLRepository) error {
			_, err := repository.PackageExists(context.Background(), 1)
			return err
		}},
		{"members", &fakeAudienceMemberQueries{membersErr: dependencyErr}, func(repository *SQLRepository) error {
			_, err := repository.ListMembers(context.Background(), 1, 1, 0)
			return err
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repository, _ := newSQLRepository(test.queries)
			if err := test.call(repository); !errors.Is(err, ErrUnavailable) || !errors.Is(err, dependencyErr) {
				t.Fatalf("error = %v, want joined unavailable dependency", err)
			}
		})
	}
}

func TestSQLRepositoryRejectsInvalidConstructionArgumentsAndRows(t *testing.T) {
	t.Parallel()
	if _, err := newSQLRepository(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("newSQLRepository(nil) error = %v", err)
	}
	if _, err := NewSQLRepository().PackageExists(context.Background(), 1); !errors.Is(err, platformport.ErrTransactionRequired) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("transaction-free PackageExists() error = %v", err)
	}
	repository, _ := newSQLRepository(&fakeAudienceMemberQueries{})
	for _, input := range []ListInput{
		{PackageID: 0, Limit: 1},
		{PackageID: 1, Limit: 0},
		{PackageID: 1, Limit: MaximumLimit + 1},
		{PackageID: 1, Limit: 1, Offset: -1},
	} {
		if _, err := repository.ListMembers(context.Background(), input.PackageID, input.Limit, input.Offset); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("ListMembers(%#v) error = %v", input, err)
		}
	}
	if _, err := repository.PackageExists(context.Background(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("PackageExists(0) error = %v", err)
	}
	badRows := &fakeAudienceMemberQueries{memberRows: []segmentdb.ListLegacyAudienceMembersRow{{
		Total: 1, CustomerID: pgtype.Int8{}, Name: pgtype.Text{String: "orphan", Valid: true}, ComputedAt: pgtype.Timestamptz{},
	}}}
	repository, _ = newSQLRepository(badRows)
	if _, err := repository.ListMembers(context.Background(), 1, 1, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid member row error = %v", err)
	}
}
