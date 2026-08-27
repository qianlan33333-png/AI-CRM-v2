package membergrid

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type publicShareStore struct {
	share       ExternalShare
	lookupErr   error
	buckets     []PublicShareBucket
	records     []PublicShareMemberRecord
	summaryErr  error
	membersErr  error
	lookupCalls int
	productID   int64
	query       StoreQuery
}

func (store *publicShareStore) CurrentExternalShare(context.Context, int64) (ExternalShare, error) {
	return ExternalShare{}, ErrUnavailable
}

func (store *publicShareStore) SetExternalShare(context.Context, SetExternalShareRecord) (ExternalShare, error) {
	return ExternalShare{}, ErrUnavailable
}

func (store *publicShareStore) LookupEnabledExternalShare(_ context.Context, shareID string) (ExternalShare, error) {
	store.lookupCalls++
	if store.lookupErr != nil {
		return ExternalShare{}, store.lookupErr
	}
	if store.share.ShareID != shareID {
		return ExternalShare{}, ErrNotFound
	}
	return store.share, nil
}

func (store *publicShareStore) SummarizePublicMembers(_ context.Context, productID int64) ([]PublicShareBucket, error) {
	store.productID = productID
	if store.summaryErr != nil {
		return nil, store.summaryErr
	}
	return append([]PublicShareBucket(nil), store.buckets...), nil
}

func (store *publicShareStore) QueryPublicMembers(_ context.Context, query StoreQuery) ([]PublicShareMemberRecord, error) {
	store.query = query
	if store.membersErr != nil {
		return nil, store.membersErr
	}
	return append([]PublicShareMemberRecord(nil), store.records...), nil
}

func TestPublicShareSummaryReturnsClosedAggregateAndSafeMemberRows(t *testing.T) {
	codec, err := NewExternalShareTokenCodec(bytes.Repeat([]byte("p"), 32))
	if err != nil {
		t.Fatal(err)
	}
	shareID := "share_abcdefghijklmnopqrstuv"
	token, err := codec.Issue(shareID)
	if err != nil {
		t.Fatal(err)
	}
	store := &publicShareStore{
		share:   ExternalShare{ServiceProductID: 91, ShareID: shareID, Enabled: true, Version: 3},
		buckets: []PublicShareBucket{{State: "removed", Count: 2}, {State: "active", Count: 7}},
		records: []PublicShareMemberRecord{{
			MemberRef: "spm_abcdefghijklmnopqrstuv",
			State:     StateActive, Source: SourceManual, StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC), DisplayName: "李同学",
		}},
	}
	cursors, err := NewCursorCodec(bytes.Repeat([]byte("c"), 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewPublicShareService(&testUnitOfWork{}, store, store, store, codec, cursors)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return stamp }
	result, err := service.Summary(context.Background(), token, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []PublicShareBucket{{State: "active", Count: 7}, {State: "expired", Count: 0}, {State: "removed", Count: 2}}
	if !reflect.DeepEqual(result.Buckets, want) || !result.AsOf.Equal(stamp) || result.Limit != MaximumLimit || result.HasMore || result.NextCursor != "" ||
		len(result.Rows) != 1 || result.Rows[0].DisplayName != "李同学" || result.Rows[0].State != "active" || result.Rows[0].Source != "manual" ||
		store.lookupCalls != 1 || store.productID != 91 || store.query.ProductID != 91 || store.query.Limit != MaximumLimit+1 {
		t.Fatalf("result=%+v calls=%d product=%d", result, store.lookupCalls, store.productID)
	}
}

func TestPublicShareSummaryFailsClosed(t *testing.T) {
	codec, err := NewExternalShareTokenCodec(bytes.Repeat([]byte("q"), 32))
	if err != nil {
		t.Fatal(err)
	}
	shareID := "share_abcdefghijklmnopqrstuv"
	token, _ := codec.Issue(shareID)
	cursors, err := NewCursorCodec(bytes.Repeat([]byte("r"), 32))
	if err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		token  string
		cursor string
		store  *publicShareStore
		want   error
	}{
		"bad token":       {token: "bad", store: &publicShareStore{}, want: ErrNotFound},
		"disabled":        {token: token, store: &publicShareStore{lookupErr: ErrNotFound}, want: ErrNotFound},
		"unknown bucket":  {token: token, store: &publicShareStore{share: ExternalShare{ServiceProductID: 1, ShareID: shareID, Enabled: true, Version: 1}, buckets: []PublicShareBucket{{State: "revoked", Count: 1}}}, want: ErrUnavailable},
		"negative bucket": {token: token, store: &publicShareStore{share: ExternalShare{ServiceProductID: 1, ShareID: shareID, Enabled: true, Version: 1}, buckets: []PublicShareBucket{{State: "active", Count: -1}}}, want: ErrUnavailable},
		"invalid cursor":  {token: token, cursor: "bad", store: &publicShareStore{share: ExternalShare{ServiceProductID: 1, ShareID: shareID, Enabled: true, Version: 1}}, want: ErrInvalidCursor},
	} {
		t.Run(name, func(t *testing.T) {
			service, serviceErr := NewPublicShareService(&testUnitOfWork{}, testCase.store, testCase.store, testCase.store, codec, cursors)
			if serviceErr != nil {
				t.Fatal(serviceErr)
			}
			if _, serviceErr = service.Summary(context.Background(), testCase.token, testCase.cursor); !errors.Is(serviceErr, testCase.want) {
				t.Fatalf("error=%v want=%v", serviceErr, testCase.want)
			}
		})
	}
}

func TestPublicShareRepositoryMapsGeneratedAggregate(t *testing.T) {
	queries := &fakeShareQueries{summary: []productdb.SummarizePublicServicePeriodMembersRow{{State: "active", MemberCount: 4}, {State: "removed", MemberCount: 1}}}
	repository := repositoryForShareQueries(queries)
	buckets, err := repository.SummarizePublicMembers(context.Background(), 9)
	if err != nil || !reflect.DeepEqual(buckets, []PublicShareBucket{{State: "active", Count: 4}, {State: "removed", Count: 1}}) || queries.summaryID != 9 {
		t.Fatalf("buckets=%+v err=%v id=%d", buckets, err, queries.summaryID)
	}
}

func TestPublicShareRepositoryMapsExplicitGeneratedProjection(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	queries := &fakeShareQueries{first: []productdb.ListPublicServicePeriodMembersFirstPageRow{{
		MemberRef: "spm_abcdefghijklmnopqrstuv", State: "active", Source: "paid_order",
		StartsAt:  pgtype.Timestamptz{Time: stamp.Add(-time.Hour), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: stamp, Valid: true}, DisplayName: "李同学",
	}}}
	repository := repositoryForShareQueries(queries)
	records, err := repository.QueryPublicMembers(context.Background(), StoreQuery{ProductID: 9, State: StateAll, Source: SourceAny, Limit: MaximumLimit + 1})
	if err != nil || len(records) != 1 || records[0].DisplayName != "李同学" || records[0].State != StateActive || records[0].Source != SourcePaidOrder || records[0].ExpiresAt != nil {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	want := productdb.ListPublicServicePeriodMembersFirstPageParams{ServiceProductID: 9, RowLimit: MaximumLimit + 1}
	if !reflect.DeepEqual(queries.firstArgs, want) {
		t.Fatalf("args=%+v want=%+v", queries.firstArgs, want)
	}
}
