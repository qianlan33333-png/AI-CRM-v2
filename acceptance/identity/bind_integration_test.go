package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var bindReceiptKey = []byte("identity-bind-receipt-key-v1-32b")

func TestIdentityBindPersistsReplayAndDivertsOtherCustomer(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	firstCustomerID, secondCustomerID := createBindCustomer(t, pool), createBindCustomer(t, pool)
	upsertIdentityForBind(t, pool, bindRef())
	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	service := newIdentityBindService(pool, recorder)
	command := bindCommand(firstCustomerID, "bind-replay-key")

	first, err := service.Bind(ctx, command)
	if err != nil || first != (identityport.BindResult{Status: identityport.BindBound, CustomerID: contactport.CustomerID(firstCustomerID)}) {
		t.Fatalf("first Bind()=%+v err=%v", first, err)
	}
	replay, err := service.Bind(ctx, command)
	if err != nil || replay != first {
		t.Fatalf("replay Bind()=%+v err=%v, want %+v", replay, err, first)
	}
	changed := command
	changed.CustomerID = contactport.CustomerID(secondCustomerID)
	if _, err = service.Bind(ctx, changed); !errors.Is(err, identityapp.ErrIdentityBindIdempotencyConflict) {
		t.Fatalf("same key changed payload error=%v", err)
	}
	sameCustomerDifferentKey := command
	sameCustomerDifferentKey.IdempotencyKey = "bind-state-replay-key"
	alreadyBound, err := service.Bind(ctx, sameCustomerDifferentKey)
	if err != nil || alreadyBound != (identityport.BindResult{Status: identityport.BindAlreadyBound, CustomerID: contactport.CustomerID(firstCustomerID)}) {
		t.Fatalf("same customer Bind()=%+v err=%v", alreadyBound, err)
	}
	otherCustomer := changed
	otherCustomer.IdempotencyKey = "bind-other-customer-key"
	rejected, err := service.Bind(ctx, otherCustomer)
	if err != nil || rejected != (identityport.BindResult{Status: identityport.BindRejected}) {
		t.Fatalf("other customer Bind()=%+v err=%v", rejected, err)
	}

	var boundCustomerID int64
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind = 'phone' AND scope = 'phone:e164'`).Scan(&boundCustomerID); err != nil || boundCustomerID != firstCustomerID {
		t.Fatalf("identity customer_id=%d err=%v, want %d", boundCustomerID, err, firstCustomerID)
	}
	var receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation = 'bind' AND state = 'completed'`).Scan(&receipts); err != nil || receipts != 3 {
		t.Fatalf("completed bind receipts=%d err=%v, want 3", receipts, err)
	}
	events := recorder.Events()
	if len(events) != 1 || events[0].Type != "identity.bound" || events[0].CustomerID != eventport.CustomerID(firstCustomerID) || events[0].IdempotencyKey == "" {
		t.Fatalf("bound events=%+v", events)
	}
}

func TestIdentityBindRollsBackReceiptAndEdgeWhenEventFails(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	customerID := createBindCustomer(t, pool)
	upsertIdentityForBind(t, pool, bindRef())
	command := bindCommand(customerID, "bind-event-failure-key")

	_, err := newIdentityBindService(pool, failingEventAppender{}).Bind(context.Background(), command)
	if err == nil {
		t.Fatal("Bind succeeded while event append failed")
	}
	var customerIDAfter *int64
	if err = pool.QueryRow(context.Background(), `SELECT customer_id FROM identities WHERE kind = 'phone' AND scope = 'phone:e164'`).Scan(&customerIDAfter); err != nil || customerIDAfter != nil {
		t.Fatalf("identity after failed Bind customer=%v err=%v", customerIDAfter, err)
	}
	var receipts int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_operation_receipts WHERE operation = 'bind'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("receipts after failed Bind=%d err=%v", receipts, err)
	}
}

func TestIdentityBindConcurrentSameKeyReturnsOriginalBoundFactOnce(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	customerID := createBindCustomer(t, pool)
	upsertIdentityForBind(t, pool, bindRef())
	secondPool, err := pgxpool.New(context.Background(), *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	firstRecorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	secondRecorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	services := []*identityapp.BindService{newIdentityBindService(pool, firstRecorder), newIdentityBindService(secondPool, secondRecorder)}
	command := bindCommand(customerID, "bind-concurrent-key")
	start := make(chan struct{})
	results := make([]identityport.BindResult, len(services))
	errs := make([]error, len(services))
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *identityapp.BindService) {
			defer wait.Done()
			<-start
			results[index], errs[index] = service.Bind(context.Background(), command)
		}(index, service)
	}
	close(start)
	wait.Wait()
	want := identityport.BindResult{Status: identityport.BindBound, CustomerID: contactport.CustomerID(customerID)}
	for index := range results {
		if errs[index] != nil || results[index] != want {
			t.Fatalf("concurrent Bind[%d]=%+v err=%v, want %+v", index, results[index], errs[index], want)
		}
	}
	if len(firstRecorder.Events())+len(secondRecorder.Events()) != 1 {
		t.Fatalf("bound events=%d, want 1", len(firstRecorder.Events())+len(secondRecorder.Events()))
	}
	var receipts, boundEdges int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_operation_receipts WHERE operation = 'bind'`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("bind receipts=%d err=%v", receipts, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identities WHERE customer_id = $1::bigint`, customerID).Scan(&boundEdges); err != nil || boundEdges != 1 {
		t.Fatalf("bound edges=%d err=%v", boundEdges, err)
	}
}

func TestIdentityBindVerifiedUnionIDAutoMergePreservesContactFactsAndReplaysReceipt(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	primaryID, mergedID := createBindCustomer(t, pool), createBindCustomer(t, pool)
	seedVerifiedWeComIdentity(t, pool, primaryID, "primary")
	unionValue := seedVerifiedUnionID(t, pool, mergedID, "merge")
	primaryTag, mergedTag := seedMergeTagsAndTimeline(t, pool, primaryID, mergedID)
	service := newIdentityBindService(pool, eventstore.NewAppender())
	command := identityport.BindCommand{
		CustomerID: contactport.CustomerID(primaryID),
		Ref: identityport.IDRef{
			Kind: identityport.KindUnionID, Scope: "wechat-open-platform:acceptance", Value: unionValue,
			Assurance: identityport.AssuranceVerified, Source: "wecom.callback",
		},
		Actor: "acceptance:identity-i4a", IdempotencyKey: "bind-unionid-merge",
	}

	result, err := service.Bind(ctx, command)
	if err != nil || result.Status != identityport.BindMerged || result.CustomerID != contactport.CustomerID(primaryID) ||
		result.PrimaryCustomerID != contactport.CustomerID(primaryID) || result.MergeAuditID <= 0 {
		t.Fatalf("Bind verified unionid result=%+v err=%v", result, err)
	}
	replay, err := service.Bind(ctx, command)
	if err != nil || replay != result {
		t.Fatalf("Bind verified unionid replay=%+v err=%v, want %+v", replay, err, result)
	}

	var mergedDeleted bool
	if err = pool.QueryRow(ctx, `SELECT is_deleted FROM customers WHERE id=$1::bigint`, mergedID).Scan(&mergedDeleted); err != nil || !mergedDeleted {
		t.Fatalf("merged customer deleted=%t err=%v", mergedDeleted, err)
	}
	for _, tagID := range []int64{primaryTag, mergedTag} {
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_tags WHERE customer_id=$1::bigint AND tag_id=$2::bigint`, primaryID, tagID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("primary tag=%d count=%d err=%v", tagID, count, err)
		}
	}
	var timelineCount, lineageCount, auditCount, mergeEvents, receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_events WHERE customer_id=$1::bigint`, mergedID).Scan(&timelineCount); err != nil || timelineCount != 1 {
		t.Fatalf("merged timeline count=%d err=%v", timelineCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merge_lineage WHERE merged_customer_id=$1::bigint AND primary_customer_id=$2::bigint`, mergedID, primaryID).Scan(&lineageCount); err != nil || lineageCount != 1 {
		t.Fatalf("lineage count=%d err=%v", lineageCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges WHERE id=$1::bigint AND primary_customer_id=$2::bigint AND merged_customer_id=$3::bigint AND mode='auto' AND policy_version=$4::text`, result.MergeAuditID, primaryID, mergedID, identityport.MergePolicyVerifiedUnionIDUniqueWeCom).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("merge audit count=%d err=%v", auditCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type='customer.merged' AND idempotency_key=$1::text`, fmt.Sprintf("customer.merged:%d", result.MergeAuditID)).Scan(&mergeEvents); err != nil || mergeEvents != 1 {
		t.Fatalf("merge events=%d err=%v", mergeEvents, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='bind' AND result_status='merged' AND result_merge_audit_id=$1::bigint`, result.MergeAuditID).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("merged receipts=%d err=%v", receipts, err)
	}
	var unionCustomerID int64
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind='unionid' AND scope='wechat-open-platform:acceptance' AND normalized_value=$1::text`, unionValue).Scan(&unionCustomerID); err != nil || unionCustomerID != primaryID {
		t.Fatalf("unionid customer=%d err=%v, want primary=%d", unionCustomerID, err, primaryID)
	}
}

func TestIdentityBindVerifiedUnionIDAutoMergeRollsBackWhenMergedEventFails(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	primaryID, mergedID := createBindCustomer(t, pool), createBindCustomer(t, pool)
	seedVerifiedWeComIdentity(t, pool, primaryID, "rollback-primary")
	unionValue := seedVerifiedUnionID(t, pool, mergedID, "rollback-merge")
	_, mergedTag := seedMergeTagsAndTimeline(t, pool, primaryID, mergedID)
	service := newIdentityBindService(pool, failingEventAppender{})

	_, err := service.Bind(ctx, identityport.BindCommand{
		CustomerID: contactport.CustomerID(primaryID),
		Ref:        identityport.IDRef{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:acceptance", Value: unionValue, Assurance: identityport.AssuranceVerified, Source: "wecom.callback"},
		Actor:      "acceptance:identity-i4a", IdempotencyKey: "bind-unionid-rollback",
	})
	if err == nil {
		t.Fatal("Bind succeeded while customer.merged append failed")
	}
	var mergedDeleted, tagCount bool
	if err = pool.QueryRow(ctx, `SELECT is_deleted FROM customers WHERE id=$1::bigint`, mergedID).Scan(&mergedDeleted); err != nil || mergedDeleted {
		t.Fatalf("rolled-back merged deleted=%t err=%v", mergedDeleted, err)
	}
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_tags WHERE customer_id=$1::bigint AND tag_id=$2::bigint)`, primaryID, mergedTag).Scan(&tagCount); err != nil || tagCount {
		t.Fatalf("rolled-back copied tag=%t err=%v", tagCount, err)
	}
	var unionCustomerID *int64
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind='unionid' AND scope='wechat-open-platform:acceptance' AND normalized_value=$1::text`, unionValue).Scan(&unionCustomerID); err != nil || unionCustomerID == nil || *unionCustomerID != mergedID {
		t.Fatalf("rolled-back union customer=%v err=%v, want=%d", unionCustomerID, err, mergedID)
	}
	var lineageCount, auditCount, receiptCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merge_lineage WHERE merged_customer_id=$1::bigint`, mergedID).Scan(&lineageCount); err != nil || lineageCount != 0 {
		t.Fatalf("rolled-back lineage=%d err=%v", lineageCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges WHERE merged_customer_id=$1::bigint`, mergedID).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("rolled-back audit=%d err=%v", auditCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='bind'`).Scan(&receiptCount); err != nil || receiptCount != 0 {
		t.Fatalf("rolled-back receipts=%d err=%v", receiptCount, err)
	}
}

func TestIdentityBindConcurrentVerifiedUnionIDMergeCreatesOneAuditAndOneEvent(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	primaryID, mergedID := createBindCustomer(t, pool), createBindCustomer(t, pool)
	seedVerifiedWeComIdentity(t, pool, primaryID, "concurrent-primary")
	unionValue := seedVerifiedUnionID(t, pool, mergedID, "concurrent-merge")
	secondPool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	services := []*identityapp.BindService{
		newIdentityBindService(pool, eventstore.NewAppender()),
		newIdentityBindService(secondPool, eventstore.NewAppender()),
	}
	command := identityport.BindCommand{
		CustomerID: contactport.CustomerID(primaryID),
		Ref:        identityport.IDRef{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:acceptance", Value: unionValue, Assurance: identityport.AssuranceVerified, Source: "wecom.callback"},
		Actor:      "acceptance:identity-i4a", IdempotencyKey: "bind-unionid-concurrent",
	}
	start := make(chan struct{})
	results := make([]identityport.BindResult, len(services))
	errs := make([]error, len(services))
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *identityapp.BindService) {
			defer wait.Done()
			<-start
			results[index], errs[index] = service.Bind(ctx, command)
		}(index, service)
	}
	close(start)
	wait.Wait()
	for index, result := range results {
		if errs[index] != nil || result.Status != identityport.BindMerged || result.CustomerID != contactport.CustomerID(primaryID) || result.PrimaryCustomerID != contactport.CustomerID(primaryID) || result.MergeAuditID <= 0 {
			t.Fatalf("concurrent result[%d]=%+v err=%v", index, result, errs[index])
		}
		if result != results[0] {
			t.Fatalf("concurrent replay result[%d]=%+v want=%+v", index, result, results[0])
		}
	}
	var auditCount, mergeEventCount, receiptCount, orphanedIdentityCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges WHERE primary_customer_id=$1::bigint AND merged_customer_id=$2::bigint`, primaryID, mergedID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("concurrent audits=%d err=%v", auditCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type='customer.merged' AND idempotency_key=$1::text`, fmt.Sprintf("customer.merged:%d", results[0].MergeAuditID)).Scan(&mergeEventCount); err != nil || mergeEventCount != 1 {
		t.Fatalf("concurrent merge events=%d err=%v", mergeEventCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='bind' AND result_status='merged'`).Scan(&receiptCount); err != nil || receiptCount != 1 {
		t.Fatalf("concurrent merged receipts=%d err=%v", receiptCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identities WHERE customer_id=$1::bigint`, mergedID).Scan(&orphanedIdentityCount); err != nil || orphanedIdentityCount != 0 {
		t.Fatalf("concurrent orphaned identities=%d err=%v", orphanedIdentityCount, err)
	}
}

func TestIdentityBindVerifiedPhoneDoesNotAutoMergeInI4A(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	primaryID, existingID := createBindCustomer(t, pool), createBindCustomer(t, pool)
	seedVerifiedWeComIdentity(t, pool, primaryID, "phone-primary")
	phoneValue := "+8613800138000"
	if _, err := pool.Exec(ctx, `
INSERT INTO identities (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'phone', 'phone:e164', $2::text, 1, 'verified', 'wecom.callback', decode('20112233445566778899aabbccddeeff', 'hex'), 1, now())`, existingID, phoneValue); err != nil {
		t.Fatal(err)
	}
	result, err := newIdentityBindService(pool, eventstore.NewAppender()).Bind(ctx, identityport.BindCommand{
		CustomerID: contactport.CustomerID(primaryID),
		Ref:        identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: phoneValue, Assurance: identityport.AssuranceVerified, Source: "wecom.callback"},
		Actor:      "acceptance:identity-i4a", IdempotencyKey: "bind-verified-phone-i4a",
	})
	if err != nil || result != (identityport.BindResult{Status: identityport.BindRejected}) {
		t.Fatalf("verified phone I4A result=%+v err=%v", result, err)
	}
	var auditCount, lineageCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merges`).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("verified phone audit count=%d err=%v", auditCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_merge_lineage WHERE (merged_customer_id=$1::bigint AND primary_customer_id=$2::bigint) OR (merged_customer_id=$2::bigint AND primary_customer_id=$1::bigint)`, primaryID, existingID).Scan(&lineageCount); err != nil || lineageCount != 0 {
		t.Fatalf("verified phone lineage count=%d err=%v", lineageCount, err)
	}
}

func newIdentityBindService(pool *pgxpool.Pool, events eventport.Appender) *identityapp.BindService {
	return identityapp.NewBindServiceWithMergePort(platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), contactstore.NewMergePortRepository(), events, bindReceiptKey)
}

func seedVerifiedWeComIdentity(t *testing.T, pool *pgxpool.Pool, customerID int64, suffix string) {
	t.Helper()
	value := fmt.Sprintf("wecom-%s-%d", suffix, time.Now().UnixNano())
	if _, err := pool.Exec(context.Background(), `
INSERT INTO identities (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'wecom_external_userid', 'wecom-corp:acceptance', $2::text, 1, 'verified', 'wecom.callback', decode('00112233445566778899aabbccddeeff', 'hex'), 1, now())`, customerID, value); err != nil {
		t.Fatalf("seed verified WeCom identity: %v", err)
	}
}

func seedVerifiedUnionID(t *testing.T, pool *pgxpool.Pool, customerID int64, suffix string) string {
	t.Helper()
	value := fmt.Sprintf("union-%s-%d", suffix, time.Now().UnixNano())
	if _, err := pool.Exec(context.Background(), `
INSERT INTO identities (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'unionid', 'wechat-open-platform:acceptance', $2::text, 1, 'verified', 'wecom.callback', decode('10112233445566778899aabbccddeeff', 'hex'), 1, now())`, customerID, value); err != nil {
		t.Fatalf("seed verified unionid: %v", err)
	}
	return value
}

func seedMergeTagsAndTimeline(t *testing.T, pool *pgxpool.Pool, primaryID, mergedID int64) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	prefix := fmt.Sprintf("identity-i4a-%d", time.Now().UnixNano())
	primaryTag, err := contactfixture.CreateTag(ctx, tx, prefix+"-primary")
	if err != nil {
		t.Fatalf("seed primary tag: %v", err)
	}
	mergedTag, err := contactfixture.CreateTag(ctx, tx, prefix+"-merged")
	if err != nil {
		t.Fatalf("seed merged tag: %v", err)
	}
	if err := contactfixture.AttachTag(ctx, tx, primaryID, primaryTag, "acceptance"); err != nil {
		t.Fatalf("seed merge tags: %v", err)
	}
	if err := contactfixture.AttachTag(ctx, tx, mergedID, mergedTag, "acceptance"); err != nil {
		t.Fatalf("seed merge tags: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"origin": "merged-customer"})
	if err := contactfixture.AppendTimelineEvent(ctx, tx, mergedID, "acceptance.identity.merge_origin", payload, "acceptance"); err != nil {
		t.Fatalf("seed merged timeline: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return primaryTag, mergedTag
}

func bindRef() identityport.IDRef {
	return identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 ", Assurance: identityport.AssuranceDeclared, Source: "admin"}
}

func bindCommand(customerID int64, idempotencyKey string) identityport.BindCommand {
	return identityport.BindCommand{CustomerID: contactport.CustomerID(customerID), Ref: bindRef(), Actor: "acceptance:identity-bind", IdempotencyKey: idempotencyKey}
}

func upsertIdentityForBind(t *testing.T, pool *pgxpool.Pool, ref identityport.IDRef) {
	t.Helper()
	result, err := newIdentityUpsertService(pool, eventstore.NewAppender()).Upsert(context.Background(), ref)
	if err != nil || !result.Created || result.IdentityID <= 0 {
		t.Fatalf("upsert identity=%+v err=%v", result, err)
	}
}

func createBindCustomer(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID
}
