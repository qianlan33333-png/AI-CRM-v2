package membergrid

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestPublicShareSummarySQLIsAggregateOnly(t *testing.T) {
	lower := strings.ToLower(summarizePublicMembersSQL)
	for _, forbidden := range []string{"customers", "customer_id", "member_ref", "display_name", "mobile", "unionid", "external_userid", "remark", "alliance", "source", "starts_at", "expires_at", "updated_at"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("public summary SQL exposes %s: %s", forbidden, summarizePublicMembersSQL)
		}
	}
	if !strings.Contains(lower, "count(*)") || !strings.Contains(lower, "group by m.state") {
		t.Fatalf("public summary is not aggregate-only: %s", summarizePublicMembersSQL)
	}
	executor := &fakeSQLExecutor{rows: &fakeSQLRows{rows: [][]any{{"active", int64(4)}, {"removed", int64(1)}}}}
	repository := repositoryForExecutor(executor)
	buckets, err := repository.SummarizePublicMembers(context.Background(), 9)
	if err != nil || !reflect.DeepEqual(buckets, []PublicShareBucket{{State: "active", Count: 4}, {State: "removed", Count: 1}}) || !reflect.DeepEqual(executor.queryArgs, []any{int64(9)}) || !executor.rows.closed {
		t.Fatalf("buckets=%+v err=%v args=%v closed=%v", buckets, err, executor.queryArgs, executor.rows.closed)
	}
}

func TestPublicShareMemberSQLAndDTOUseExplicitAllowlist(t *testing.T) {
	lower := strings.ToLower(publicMemberProjection)
	for _, forbidden := range []string{"\n  m.customer_id,", "mobile", "unionid", "external_userid", "remark", "alliance", "version"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("public member projection exposes %s: %s", forbidden, publicMemberProjection)
		}
	}
	stamp := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	executor := &fakeSQLExecutor{rows: &fakeSQLRows{rows: [][]any{{
		"spm_abcdefghijklmnopqrstuv", "active", "paid_order", stamp.Add(-time.Hour), time.Unix(0, 0).UTC(), false, stamp, "李同学",
	}}}}
	repository := repositoryForExecutor(executor)
	records, err := repository.QueryPublicMembers(context.Background(), StoreQuery{ProductID: 9, State: StateAll, Source: SourceAny, Limit: MaximumLimit + 1})
	if err != nil || len(records) != 1 || records[0].DisplayName != "李同学" || records[0].State != StateActive || records[0].Source != SourcePaidOrder || records[0].ExpiresAt != nil {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	if executor.querySQL != firstPublicMembersPageSQL || !reflect.DeepEqual(executor.queryArgs, []any{int64(9), MaximumLimit + 1}) || !executor.rows.closed {
		t.Fatalf("sql/args/closed=%q/%v/%v", executor.querySQL, executor.queryArgs, executor.rows.closed)
	}
}
