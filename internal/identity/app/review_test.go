package app

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type reviewTestStore struct {
	records       []MergeReviewHistoryRecord
	receipt       MergeReviewReceipt
	locked        MergeReviewRecord
	roots         []contactport.CustomerID
	err           error
	listStatus    identityport.MergeReviewStatus
	listAfter     int64
	listLimit     int32
	reserveCalls  int
	lockCalls     int
	rebindCalls   int
	auditCalls    int
	resolveCalls  int
	completeCalls int
	audit         ManualMergeAudit
	resolved      MergeReviewRecord
}

func (store *reviewTestStore) ListMergeReviewsByStatus(_ context.Context, status identityport.MergeReviewStatus, after int64, limit int32) ([]MergeReviewHistoryRecord, error) {
	store.listStatus, store.listAfter, store.listLimit = status, after, limit
	return append([]MergeReviewHistoryRecord(nil), store.records...), store.err
}

func (store *reviewTestStore) ReserveMergeReviewReceipt(_ context.Context, _ string, _, _ []byte) (MergeReviewReceipt, error) {
	store.reserveCalls++
	return store.receipt, store.err
}

func (store *reviewTestStore) LockMergeReview(_ context.Context, _ int64) (MergeReviewRecord, error) {
	store.lockCalls++
	return store.locked, store.err
}

func (store *reviewTestStore) LockActiveMergeReviewCustomers(_ context.Context, _ []contactport.CustomerID) ([]contactport.CustomerID, error) {
	return append([]contactport.CustomerID(nil), store.roots...), store.err
}

func (store *reviewTestStore) RebindIdentitiesForCustomerMerge(_ context.Context, _, _ contactport.CustomerID) error {
	store.rebindCalls++
	return store.err
}

func (store *reviewTestStore) InsertManualCustomerMergeAudit(_ context.Context, audit ManualMergeAudit) (int64, error) {
	store.auditCalls++
	store.audit = audit
	return 17, store.err
}

func (store *reviewTestStore) ResolveMergeReview(_ context.Context, _, _ int64, status identityport.MergeReviewStatus) (MergeReviewRecord, error) {
	store.resolveCalls++
	result := store.resolved
	result.Status = status
	return result, store.err
}

func (store *reviewTestStore) CompleteMergeReviewReceipt(_ context.Context, _ MergeReviewReceipt, _ MergeReviewRecord, _ int64) error {
	store.completeCalls++
	return store.err
}

func TestMergeReviewListUsesStatusBoundOpaqueCursorAndClosedFacts(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	for _, status := range []identityport.MergeReviewStatus{
		identityport.MergeReviewPending,
		identityport.MergeReviewApproved,
		identityport.MergeReviewRejected,
	} {
		t.Run(string(status), func(t *testing.T) {
			records := []MergeReviewHistoryRecord{
				historyReviewRecord(11, status),
				historyReviewRecord(12, status),
				historyReviewRecord(13, status),
			}
			store := &reviewTestStore{records: records}
			service := NewMergeReviewService(&resolveTestUoW{}, store, &bindTestContacts{}, &bindTestEvents{}, key)

			page, err := service.ListMergeReviewsByStatus(context.Background(), status, "", 2)
			if err != nil || len(page.Items) != 2 || page.NextCursor == "" || store.listStatus != status || store.listAfter != 0 || store.listLimit != 3 {
				t.Fatalf("page=%+v err=%v status=%q after=%d limit=%d", page, err, store.listStatus, store.listAfter, store.listLimit)
			}
			if page.Items[0].Status != status || page.Items[0].Scope != records[0].Scope {
				t.Fatalf("public review leaked or drifted: %+v", page.Items[0])
			}
			for _, forbidden := range []string{"IdentityFingerprint", "NormalizedValue", "Payload"} {
				if _, found := reflect.TypeOf(page.Items[0]).FieldByName(forbidden); found {
					t.Fatalf("public review exposes forbidden field %q", forbidden)
				}
			}

			store.records = nil
			page, err = service.ListMergeReviewsByStatus(context.Background(), status, page.NextCursor, 2)
			if err != nil || store.listStatus != status || store.listAfter != 12 || len(page.Items) != 0 {
				t.Fatalf("second page=%+v err=%v status=%q after=%d", page, err, store.listStatus, store.listAfter)
			}
		})
	}
}

func TestMergeReviewCursorCannotCrossStatuses(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	store := &reviewTestStore{records: []MergeReviewHistoryRecord{historyReviewRecord(11, identityport.MergeReviewPending), historyReviewRecord(12, identityport.MergeReviewPending)}}
	service := NewMergeReviewService(&resolveTestUoW{}, store, &bindTestContacts{}, &bindTestEvents{}, key)
	page, err := service.ListMergeReviews(context.Background(), "", 1)
	if err != nil || page.NextCursor == "" {
		t.Fatalf("pending page=%+v err=%v", page, err)
	}
	for _, status := range []identityport.MergeReviewStatus{identityport.MergeReviewApproved, identityport.MergeReviewRejected} {
		if _, err = service.ListMergeReviewsByStatus(context.Background(), status, page.NextCursor, 1); !errors.Is(err, ErrMergeReviewInvalid) {
			t.Fatalf("cross-status cursor status=%q err=%v", status, err)
		}
	}
}

func TestMergeReviewListFailsClosedForStatusAndResolvedTimeContradictions(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	service := NewMergeReviewService(&resolveTestUoW{}, &reviewTestStore{}, &bindTestContacts{}, &bindTestEvents{}, key)
	if _, err := service.ListMergeReviewsByStatus(context.Background(), identityport.MergeReviewStatus("other"), "", 10); !errors.Is(err, ErrMergeReviewInvalid) {
		t.Fatalf("invalid status err=%v", err)
	}

	created := time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	for _, record := range []MergeReviewHistoryRecord{
		func() MergeReviewHistoryRecord {
			r := historyReviewRecord(21, identityport.MergeReviewPending)
			r.ResolvedAt = &created
			return r
		}(),
		func() MergeReviewHistoryRecord {
			r := historyReviewRecord(22, identityport.MergeReviewApproved)
			r.ResolvedAt = nil
			return r
		}(),
		func() MergeReviewHistoryRecord {
			r := historyReviewRecord(23, identityport.MergeReviewRejected)
			before := r.CreatedAt.Add(-time.Second)
			r.ResolvedAt = &before
			return r
		}(),
	} {
		store := &reviewTestStore{records: []MergeReviewHistoryRecord{record}}
		service = NewMergeReviewService(&resolveTestUoW{}, store, &bindTestContacts{}, &bindTestEvents{}, key)
		if _, err := service.ListMergeReviewsByStatus(context.Background(), record.Status, "", 10); !errors.Is(err, ErrMergeReviewUnavailable) {
			t.Fatalf("contradictory record=%+v err=%v", record, err)
		}
	}
}

func TestMergeReviewApproveMergesAndCompletesOneTransactionFact(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	pending := validReviewRecord(key, 23)
	resolved := pending
	resolved.Status = identityport.MergeReviewApproved
	resolved.Version = 2
	resolvedAt := time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	resolved.ResolvedAt = &resolvedAt
	store := &reviewTestStore{
		receipt: MergeReviewReceipt{ID: 9}, locked: pending,
		roots: []contactport.CustomerID{42, 84}, resolved: resolved,
	}
	contacts, events := &bindTestContacts{}, &bindTestEvents{}
	service := NewMergeReviewService(&resolveTestUoW{}, store, contacts, events, key)
	command := identityport.ApproveMergeReviewCommand{
		ReviewID: 23, ExpectedVersion: 1, PrimaryCustomerID: 84,
		Reason: "运营确认同一客户", Actor: "admin:7", IdempotencyKey: "review-approve-23",
	}

	result, err := service.ApproveMergeReview(context.Background(), command)
	if err != nil || result.Status != identityport.MergeReviewApproved || result.Version != 2 || result.ResolvedAt == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(contacts.merges) != 1 || contacts.merges[0].PrimaryID != 84 || contacts.merges[0].MergedID != 42 || contacts.merges[0].Reason != command.Reason {
		t.Fatalf("merges=%+v", contacts.merges)
	}
	if store.rebindCalls != 1 || store.auditCalls != 1 || store.resolveCalls != 1 || store.completeCalls != 1 ||
		store.audit.PolicyVersion != VerifiedPhoneMergeReviewPolicy || string(store.audit.Actor) != "admin:7" {
		t.Fatalf("store calls rebind=%d audit=%d resolve=%d complete=%d audit=%+v", store.rebindCalls, store.auditCalls, store.resolveCalls, store.completeCalls, store.audit)
	}
	if len(events.events) != 1 || events.events[0].Type != "customer.merged" || events.events[0].IdempotencyKey != "customer.merged:17" {
		t.Fatalf("events=%+v", events.events)
	}
}

func TestMergeReviewRejectChangesOnlyReviewAndReceipt(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	pending := validReviewRecord(key, 24)
	resolved := pending
	resolved.Status = identityport.MergeReviewRejected
	resolved.Version = 2
	now := time.Now().UTC()
	resolved.ResolvedAt = &now
	store := &reviewTestStore{
		receipt: MergeReviewReceipt{ID: 10}, locked: pending,
		roots: []contactport.CustomerID{42, 84}, resolved: resolved,
	}
	contacts, events := &bindTestContacts{}, &bindTestEvents{}
	service := NewMergeReviewService(&resolveTestUoW{}, store, contacts, events, key)

	result, err := service.RejectMergeReview(context.Background(), identityport.RejectMergeReviewCommand{
		ReviewID: 24, ExpectedVersion: 1, Reason: "手机号已换主", Actor: "admin:8", IdempotencyKey: "review-reject-24",
	})
	if err != nil || result.Status != identityport.MergeReviewRejected || len(contacts.merges) != 0 || len(events.events) != 0 ||
		store.rebindCalls != 0 || store.auditCalls != 0 || store.resolveCalls != 1 || store.completeCalls != 1 {
		t.Fatalf("result=%+v err=%v merges=%d events=%d store=%+v", result, err, len(contacts.merges), len(events.events), store)
	}
}

func TestMergeReviewFailsClosedForVersionRootAndEvidenceDrift(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	command := identityport.ApproveMergeReviewCommand{
		ReviewID: 23, ExpectedVersion: 1, PrimaryCustomerID: 42,
		Reason: "confirm", Actor: "admin:7", IdempotencyKey: "approve-23",
	}
	for _, test := range []struct {
		name   string
		mutate func(*reviewTestStore)
	}{
		{name: "version", mutate: func(store *reviewTestStore) { store.locked.Version = 2 }},
		{name: "root", mutate: func(store *reviewTestStore) { store.roots = []contactport.CustomerID{42} }},
		{name: "evidence", mutate: func(store *reviewTestStore) { store.locked.IdentityFingerprint[0] ^= 0xff }},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending := validReviewRecord(key, 23)
			store := &reviewTestStore{receipt: MergeReviewReceipt{ID: 9}, locked: pending, roots: []contactport.CustomerID{42, 84}}
			test.mutate(store)
			contacts, events := &bindTestContacts{}, &bindTestEvents{}
			_, err := NewMergeReviewService(&resolveTestUoW{}, store, contacts, events, key).ApproveMergeReview(context.Background(), command)
			if !errors.Is(err, ErrMergeReviewConflict) || len(contacts.merges) != 0 || len(events.events) != 0 || store.completeCalls != 0 {
				t.Fatalf("err=%v merges=%d events=%d complete=%d", err, len(contacts.merges), len(events.events), store.completeCalls)
			}
		})
	}
}

func TestMergeReviewRejectsInvalidCommandsBeforeUoW(t *testing.T) {
	uow := &resolveTestUoW{}
	service := NewMergeReviewService(uow, &reviewTestStore{}, &bindTestContacts{}, &bindTestEvents{}, []byte("12345678901234567890123456789012"))
	_, err := service.ApproveMergeReview(context.Background(), identityport.ApproveMergeReviewCommand{})
	if !errors.Is(err, ErrMergeReviewInvalid) || uow.calls != 0 {
		t.Fatalf("err=%v uow=%d", err, uow.calls)
	}
	if _, err = service.ListMergeReviews(context.Background(), "not+base64", 10); !errors.Is(err, ErrMergeReviewInvalid) {
		t.Fatalf("invalid cursor error=%v", err)
	}
	valid, _ := encodeMergeReviewCursor(identityport.MergeReviewPending, 1)
	decoded, _ := base64.RawURLEncoding.DecodeString(valid)
	trailing := base64.RawURLEncoding.EncodeToString(append(decoded, []byte(" trailing")...))
	if _, err = service.ListMergeReviews(context.Background(), trailing, 10); !errors.Is(err, ErrMergeReviewInvalid) {
		t.Fatalf("trailing cursor error=%v", err)
	}
}

func validReviewRecord(key []byte, id int64) MergeReviewRecord {
	normalized := "+8613800138000"
	digest := hmacDigest(key, "identity.bind.merge.review.v1\x00phone\x00phone:e164\x00"+normalized)
	return MergeReviewRecord{
		ReviewID: id, Status: identityport.MergeReviewPending, Kind: identityport.KindPhone,
		Scope: "phone:e164", NormalizedValue: normalized, IdentityID: id + 100,
		IdentityFingerprint: append([]byte(nil), digest[:16]...), FingerprintVersion: 1,
		CustomerIDs: []contactport.CustomerID{42, 84}, PolicyVersion: VerifiedPhoneMergeReviewPolicy,
		Version: 1, CreatedAt: time.Date(2026, 8, 13, 7, 0, 0, 0, time.UTC),
	}
}

func historyReviewRecord(id int64, status identityport.MergeReviewStatus) MergeReviewHistoryRecord {
	record := MergeReviewHistoryRecord{
		ReviewID: id, Status: status, Kind: identityport.KindPhone, Scope: "phone:e164",
		CustomerIDs: []contactport.CustomerID{42, 84}, Version: 1,
		CreatedAt: time.Date(2026, 8, 13, 7, 0, 0, 0, time.UTC),
	}
	if status != identityport.MergeReviewPending {
		resolved := record.CreatedAt.Add(time.Hour)
		record.ResolvedAt = &resolved
		record.Version = 2
	}
	return record
}

func TestMergeReviewCustomerIDsStaySorted(t *testing.T) {
	record := validReviewRecord([]byte("12345678901234567890123456789012"), 1)
	public, err := publicMergeReview(record)
	if err != nil || !reflect.DeepEqual(public.CustomerIDs, []contactport.CustomerID{42, 84}) {
		t.Fatalf("public=%+v err=%v", public, err)
	}
}
