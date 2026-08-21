package identity_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestMergeReviewApproveIsAtomicRedactedAndIdempotent(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	requestedID, existingID, reviewID, phone := seedPendingPhoneReview(t, pool, "approve")
	service := newMergeReviewService(pool, eventstore.NewAppender())
	mergeEventsBefore := countMergeEvents(t, pool)

	page, err := service.ListMergeReviews(ctx, "", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ReviewID != reviewID || page.Items[0].ResolvedAt != nil {
		t.Fatalf("review page=%+v err=%v", page, err)
	}
	for _, forbidden := range []string{"NormalizedValue", "IdentityFingerprint", "Payload"} {
		if _, found := reflect.TypeOf(page.Items[0]).FieldByName(forbidden); found {
			t.Fatalf("public review exposes forbidden field %q", forbidden)
		}
	}
	command := identityport.ApproveMergeReviewCommand{
		ReviewID: reviewID, ExpectedVersion: 1, PrimaryCustomerID: contactport.CustomerID(requestedID),
		Reason: "运营确认同一客户", Actor: "admin:acceptance-i7", IdempotencyKey: "review-approve-i7",
	}
	approved, err := service.ApproveMergeReview(ctx, command)
	if err != nil || approved.Status != identityport.MergeReviewApproved || approved.Version != 2 || approved.ResolvedAt == nil {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	replay, err := service.ApproveMergeReview(ctx, command)
	if err != nil || !sameReviewFact(replay, approved) {
		t.Fatalf("replay=%+v err=%v want=%+v", replay, err, approved)
	}
	pendingPage, err := service.ListMergeReviews(ctx, "", 10)
	if err != nil || len(pendingPage.Items) != 0 {
		t.Fatalf("resolved review remained pending: page=%+v err=%v", pendingPage, err)
	}
	approvedPage, err := service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewApproved, "", 10)
	if err != nil || len(approvedPage.Items) != 1 || approvedPage.Items[0].ReviewID != reviewID || approvedPage.Items[0].ResolvedAt == nil {
		t.Fatalf("approved history=%+v err=%v", approvedPage, err)
	}
	changed := command
	changed.Reason = "different decision payload"
	if _, err = service.ApproveMergeReview(ctx, changed); !errors.Is(err, identityapp.ErrMergeReviewConflict) {
		t.Fatalf("same key changed payload err=%v", err)
	}

	var state string
	var version, phoneCustomerID, auditCount, lineageCount, mergeEventCount, receiptCount int64
	if err = pool.QueryRow(ctx, `SELECT state, version FROM pending_events WHERE id=$1::bigint`, reviewID).Scan(&state, &version); err != nil || state != "approved" || version != 2 {
		t.Fatalf("review state=%q version=%d err=%v", state, version, err)
	}
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind='phone' AND normalized_value=$1::text`, phone).Scan(&phoneCustomerID); err != nil || phoneCustomerID != requestedID {
		t.Fatalf("phone customer=%d err=%v want=%d", phoneCustomerID, err, requestedID)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges WHERE primary_customer_id=$1::bigint AND merged_customer_id=$2::bigint AND mode='manual' AND policy_version=$3::text`, requestedID, existingID, identityapp.VerifiedPhoneMergeReviewPolicy).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("manual audits=%d err=%v", auditCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merge_lineage WHERE primary_customer_id=$1::bigint AND merged_customer_id=$2::bigint`, requestedID, existingID).Scan(&lineageCount); err != nil || lineageCount != 1 {
		t.Fatalf("lineage=%d err=%v", lineageCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type='customer.merged'`).Scan(&mergeEventCount); err != nil || mergeEventCount != mergeEventsBefore+1 {
		t.Fatalf("merge events=%d err=%v", mergeEventCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='merge_review_approve' AND result_pending_event_id=$1::bigint AND state='completed'`, reviewID).Scan(&receiptCount); err != nil || receiptCount != 1 {
		t.Fatalf("approve receipts=%d err=%v", receiptCount, err)
	}
}

func TestMergeReviewRejectNeverMergesCustomers(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	requestedID, existingID, reviewID, phone := seedPendingPhoneReview(t, pool, "reject")
	service := newMergeReviewService(pool, eventstore.NewAppender())
	mergeEventsBefore := countMergeEvents(t, pool)
	command := identityport.RejectMergeReviewCommand{
		ReviewID: reviewID, ExpectedVersion: 1, Reason: "手机号已经换主",
		Actor: "admin:acceptance-i7", IdempotencyKey: "review-reject-i7",
	}
	rejected, err := service.RejectMergeReview(ctx, command)
	if err != nil || rejected.Status != identityport.MergeReviewRejected || rejected.Version != 2 || rejected.ResolvedAt == nil {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
	replay, err := service.RejectMergeReview(ctx, command)
	if err != nil || !sameReviewFact(replay, rejected) {
		t.Fatalf("reject replay=%+v err=%v", replay, err)
	}
	pendingPage, err := service.ListMergeReviews(ctx, "", 10)
	if err != nil || len(pendingPage.Items) != 0 {
		t.Fatalf("rejected review remained pending: page=%+v err=%v", pendingPage, err)
	}
	rejectedPage, err := service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewRejected, "", 10)
	if err != nil || len(rejectedPage.Items) != 1 || rejectedPage.Items[0].ReviewID != reviewID || rejectedPage.Items[0].ResolvedAt == nil {
		t.Fatalf("rejected history=%+v err=%v", rejectedPage, err)
	}
	var phoneCustomerID, audits, lineage, mergeEvents int64
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind='phone' AND normalized_value=$1::text`, phone).Scan(&phoneCustomerID); err != nil || phoneCustomerID != existingID {
		t.Fatalf("rejected phone customer=%d err=%v want=%d", phoneCustomerID, err, existingID)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("rejected audits=%d err=%v", audits, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merge_lineage WHERE (primary_customer_id=$1::bigint AND merged_customer_id=$2::bigint) OR (primary_customer_id=$2::bigint AND merged_customer_id=$1::bigint)`, requestedID, existingID).Scan(&lineage); err != nil || lineage != 0 {
		t.Fatalf("rejected lineage=%d err=%v", lineage, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type='customer.merged'`).Scan(&mergeEvents); err != nil || mergeEvents != mergeEventsBefore {
		t.Fatalf("rejected merge events=%d err=%v", mergeEvents, err)
	}
}

func TestMergeReviewHistoryPartitionsCursorByStatus(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	_, _, firstPending, _ := seedPendingPhoneReview(t, pool, "history-pending-1")
	_, _, secondPending, _ := seedPendingPhoneReview(t, pool, "history-pending-2")
	_, _, approvedID, _ := seedPendingPhoneReview(t, pool, "history-approved")
	_, _, rejectedID, _ := seedPendingPhoneReview(t, pool, "history-rejected")
	resolvedAt := time.Now().UTC().Add(time.Minute)
	for id, status := range map[int64]identityport.MergeReviewStatus{
		approvedID: identityport.MergeReviewApproved,
		rejectedID: identityport.MergeReviewRejected,
	} {
		if _, err := pool.Exec(ctx, `UPDATE pending_events SET state=$2::text, version=2, resolved_at=$3::timestamptz WHERE id=$1::bigint`, id, string(status), resolvedAt); err != nil {
			t.Fatal(err)
		}
	}
	service := newMergeReviewService(pool, eventstore.NewAppender())

	pendingPage, err := service.ListMergeReviews(ctx, "", 1)
	if err != nil || len(pendingPage.Items) != 1 || pendingPage.NextCursor == "" || pendingPage.Items[0].ReviewID != firstPending {
		t.Fatalf("pending first page=%+v err=%v ids=%d,%d", pendingPage, err, firstPending, secondPending)
	}
	nextPending, err := service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewPending, pendingPage.NextCursor, 1)
	if err != nil || len(nextPending.Items) != 1 || nextPending.Items[0].ReviewID != secondPending || nextPending.NextCursor != "" {
		t.Fatalf("pending second page=%+v err=%v", nextPending, err)
	}
	for _, status := range []identityport.MergeReviewStatus{identityport.MergeReviewApproved, identityport.MergeReviewRejected} {
		if _, err = service.ListMergeReviewsByStatus(ctx, status, pendingPage.NextCursor, 1); !errors.Is(err, identityapp.ErrMergeReviewInvalid) {
			t.Fatalf("pending cursor accepted for %q: %v", status, err)
		}
	}
	approvedPage, err := service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewApproved, "", 10)
	if err != nil || len(approvedPage.Items) != 1 || approvedPage.Items[0].ReviewID != approvedID || approvedPage.Items[0].ResolvedAt == nil {
		t.Fatalf("approved page=%+v err=%v", approvedPage, err)
	}
	rejectedPage, err := service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewRejected, "", 10)
	if err != nil || len(rejectedPage.Items) != 1 || rejectedPage.Items[0].ReviewID != rejectedID || rejectedPage.Items[0].ResolvedAt == nil {
		t.Fatalf("rejected page=%+v err=%v", rejectedPage, err)
	}
}

func TestMergeReviewHistoryFailsClosedForResolvedTimeBeforeCreation(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	_, _, reviewID, _ := seedPendingPhoneReview(t, pool, "history-contradiction")
	service := newMergeReviewService(pool, eventstore.NewAppender())

	createdAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	before := createdAt.Add(-time.Second)
	if _, err := pool.Exec(ctx, `UPDATE pending_events SET state='rejected', version=2, created_at=$2::timestamptz, resolved_at=$3::timestamptz WHERE id=$1::bigint`, reviewID, createdAt, before); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewRejected, "", 10); !errors.Is(err, identityapp.ErrMergeReviewUnavailable) {
		t.Fatalf("contradictory resolved time err=%v", err)
	}
}

func TestMergeReviewApproveRollsBackEveryFactWhenEventFails(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	requestedID, existingID, reviewID, phone := seedPendingPhoneReview(t, pool, "rollback")
	service := newMergeReviewService(pool, failingEventAppender{})
	_, err := service.ApproveMergeReview(ctx, identityport.ApproveMergeReviewCommand{
		ReviewID: reviewID, ExpectedVersion: 1, PrimaryCustomerID: contactport.CustomerID(requestedID),
		Reason: "confirm", Actor: "admin:acceptance-i7", IdempotencyKey: "review-rollback-i7",
	})
	if err == nil {
		t.Fatal("approve succeeded while customer.merged append failed")
	}
	var state string
	var version, phoneCustomerID, audits, lineage, receipts int64
	if err = pool.QueryRow(ctx, `SELECT state, version FROM pending_events WHERE id=$1::bigint`, reviewID).Scan(&state, &version); err != nil || state != "pending" || version != 1 {
		t.Fatalf("rolled back review state=%q version=%d err=%v", state, version, err)
	}
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind='phone' AND normalized_value=$1::text`, phone).Scan(&phoneCustomerID); err != nil || phoneCustomerID != existingID {
		t.Fatalf("rolled back phone customer=%d err=%v", phoneCustomerID, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("rolled back audits=%d err=%v", audits, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merge_lineage WHERE merged_customer_id=$1::bigint`, existingID).Scan(&lineage); err != nil || lineage != 0 {
		t.Fatalf("rolled back lineage=%d err=%v", lineage, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='merge_review_approve'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("rolled back receipts=%d err=%v", receipts, err)
	}
}

func TestMergeReviewConcurrentApproveReturnsOneManualMergeFact(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	requestedID, existingID, reviewID, _ := seedPendingPhoneReview(t, pool, "concurrent")
	mergeEventsBefore := countMergeEvents(t, pool)
	secondPool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	command := identityport.ApproveMergeReviewCommand{
		ReviewID: reviewID, ExpectedVersion: 1, PrimaryCustomerID: contactport.CustomerID(requestedID),
		Reason: "confirm", Actor: "admin:acceptance-i7", IdempotencyKey: "review-concurrent-i7",
	}
	services := []*identityapp.MergeReviewService{newMergeReviewService(pool, eventstore.NewAppender()), newMergeReviewService(secondPool, eventstore.NewAppender())}
	results := make([]identityport.MergeReview, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *identityapp.MergeReviewService) {
			defer wait.Done()
			<-start
			results[index], errs[index] = service.ApproveMergeReview(ctx, command)
		}(index, service)
	}
	close(start)
	wait.Wait()
	for index := range results {
		if errs[index] != nil || !sameReviewFact(results[index], results[0]) || results[index].Status != identityport.MergeReviewApproved {
			t.Fatalf("result[%d]=%+v err=%v first=%+v", index, results[index], errs[index], results[0])
		}
	}
	var audits, lineage, events, receipts int64
	for query, target := range map[string]*int64{
		`SELECT count(*) FROM customer_merges WHERE mode='manual'`:                                &audits,
		`SELECT count(*) FROM customer_merge_lineage WHERE merged_customer_id=$1::bigint`:         &lineage,
		`SELECT count(*) FROM event_log WHERE event_type='customer.merged'`:                       &events,
		`SELECT count(*) FROM identity_operation_receipts WHERE operation='merge_review_approve'`: &receipts,
	} {
		var scanErr error
		if strings.Contains(query, "$1") {
			scanErr = pool.QueryRow(ctx, query, existingID).Scan(target)
		} else {
			scanErr = pool.QueryRow(ctx, query).Scan(target)
		}
		if scanErr != nil {
			t.Fatal(scanErr)
		}
	}
	if audits != 1 || lineage != 1 || events != mergeEventsBefore+1 || receipts != 1 {
		t.Fatalf("audits=%d lineage=%d events=%d receipts=%d", audits, lineage, events, receipts)
	}
}

func countMergeEvents(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var count int64
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM event_log WHERE event_type='customer.merged'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func seedPendingPhoneReview(t *testing.T, pool *pgxpool.Pool, suffix string) (requestedID, existingID, reviewID int64, phone string) {
	t.Helper()
	requestedID, existingID = createBindCustomer(t, pool), createBindCustomer(t, pool)
	phone = fmt.Sprintf("+86138%08d", time.Now().UnixNano()%100000000)
	seedBoundVerifiedPhone(t, pool, existingID, phone)
	result, err := newIdentityBindService(pool, eventstore.NewAppender()).Bind(context.Background(), identityport.BindCommand{
		CustomerID: contactport.CustomerID(requestedID),
		Ref:        identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: phone, Assurance: identityport.AssuranceVerified, Source: "wecom.callback"},
		Actor:      "acceptance:identity-i7", IdempotencyKey: "bind-review-i7-" + suffix,
	})
	if err != nil || result.Status != identityport.BindManualReview || result.ReviewID <= 0 {
		t.Fatalf("seed review result=%+v err=%v", result, err)
	}
	return requestedID, existingID, result.ReviewID, phone
}

func newMergeReviewService(pool *pgxpool.Pool, events eventport.Appender) *identityapp.MergeReviewService {
	return identityapp.NewMergeReviewService(
		platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), contactstore.NewMergePortRepository(), events, bindReceiptKey,
	)
}

func sameReviewFact(left, right identityport.MergeReview) bool {
	return left.ReviewID == right.ReviewID && left.Status == right.Status && left.Kind == right.Kind && left.Scope == right.Scope &&
		left.Version == right.Version &&
		len(left.CustomerIDs) == 2 && len(right.CustomerIDs) == 2 && left.CustomerIDs[0] == right.CustomerIDs[0] && left.CustomerIDs[1] == right.CustomerIDs[1] &&
		left.CreatedAt.Equal(right.CreatedAt) && left.ResolvedAt != nil && right.ResolvedAt != nil && left.ResolvedAt.Equal(*right.ResolvedAt)
}
