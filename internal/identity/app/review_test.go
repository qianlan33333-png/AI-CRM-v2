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
	records       []MergeReviewRecord
	receipt       MergeReviewReceipt
	locked        MergeReviewRecord
	roots         []contactport.CustomerID
	err           error
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

func (store *reviewTestStore) ListPendingMergeReviews(_ context.Context, after int64, limit int32) ([]MergeReviewRecord, error) {
	store.listAfter, store.listLimit = after, limit
	return append([]MergeReviewRecord(nil), store.records...), store.err
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

func TestMergeReviewListUsesOpaqueCursorAndNeverExposesIdentityValue(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	records := []MergeReviewRecord{validReviewRecord(key, 11), validReviewRecord(key, 12), validReviewRecord(key, 13)}
	store := &reviewTestStore{records: records}
	service := NewMergeReviewService(&resolveTestUoW{}, store, &bindTestContacts{}, &bindTestEvents{}, key)

	page, err := service.ListMergeReviews(context.Background(), "", 2)
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" || store.listAfter != 0 || store.listLimit != 3 {
		t.Fatalf("page=%+v err=%v after=%d limit=%d", page, err, store.listAfter, store.listLimit)
	}
	if page.Items[0].IdentityFingerprint == records[0].NormalizedValue || page.Items[0].IdentityFingerprint != "hmac-sha256-v1:"+base64Fingerprint(records[0].IdentityFingerprint) {
		t.Fatalf("public fingerprint=%q", page.Items[0].IdentityFingerprint)
	}

	store.records = nil
	page, err = service.ListMergeReviews(context.Background(), page.NextCursor, 2)
	if err != nil || store.listAfter != 12 || len(page.Items) != 0 {
		t.Fatalf("second page=%+v err=%v after=%d", page, err, store.listAfter)
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
	valid, _ := encodeMergeReviewCursor(1)
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

func base64Fingerprint(value []byte) string {
	result, err := mergeReviewFingerprint(MergeReviewRecord{IdentityFingerprint: value, FingerprintVersion: 1})
	if err != nil {
		panic(err)
	}
	return result[len("hmac-sha256-v1:"):]
}

func TestMergeReviewCustomerIDsStaySorted(t *testing.T) {
	record := validReviewRecord([]byte("12345678901234567890123456789012"), 1)
	public, err := publicMergeReview(record)
	if err != nil || !reflect.DeepEqual(public.CustomerIDs, []contactport.CustomerID{42, 84}) {
		t.Fatalf("public=%+v err=%v", public, err)
	}
}
