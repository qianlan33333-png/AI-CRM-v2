package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type couponTestUOW struct{}

func (couponTestUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, couponTestTxKey{}, true))
}

type couponTestTxKey struct{}

type couponTestEvents struct{ rows []eventport.Event }

func (e *couponTestEvents) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	if in, _ := ctx.Value(couponTestTxKey{}).(bool); !in {
		return 0, errors.New("event escaped coupon transaction")
	}
	e.rows = append(e.rows, event)
	return eventport.EventID(len(e.rows)), nil
}

type couponTestProducts struct{}

func (couponTestProducts) Get(_ context.Context, id productport.ID) (productport.Product, error) {
	return productport.Product{ID: id, Currency: "CNY", PriceMinor: 99999}, nil
}

type couponTestStore struct {
	coupons  map[couponport.ID]couponport.Coupon
	claims   []couponport.Claim
	receipts map[string]Receipt
	payment  map[[32]byte]int64
	sidebar  map[[32]byte]int64
	nextID   couponport.ID
	updates  int
}

func newCouponTestStore(items ...couponport.Coupon) *couponTestStore {
	store := &couponTestStore{coupons: map[couponport.ID]couponport.Coupon{}, receipts: map[string]Receipt{}, payment: map[[32]byte]int64{}, sidebar: map[[32]byte]int64{}, nextID: 100}
	for _, item := range items {
		store.coupons[item.ID] = item
		if item.ID >= store.nextID {
			store.nextID = item.ID + 1
		}
	}
	return store
}

func (s *couponTestStore) List(_ context.Context, limit, offset int32, search, status string) ([]couponport.Coupon, error) {
	items := make([]couponport.Coupon, 0, len(s.coupons))
	for _, item := range s.coupons {
		if (search == "" || item.Name == search) && (status == "" || item.Status == status) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	start := int(offset)
	if start > len(items) {
		start = len(items)
	}
	end := start + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return append([]couponport.Coupon(nil), items[start:end]...), nil
}
func (s *couponTestStore) Count(_ context.Context, search, status string) (int64, error) {
	items, _ := s.List(context.Background(), MaximumLimit, 0, search, status)
	return int64(len(items)), nil
}
func (s *couponTestStore) Get(_ context.Context, id couponport.ID) (couponport.Coupon, error) {
	item, ok := s.coupons[id]
	if !ok {
		return couponport.Coupon{}, ErrNotFound
	}
	return item, nil
}
func (s *couponTestStore) Lock(ctx context.Context, id couponport.ID) (couponport.Coupon, error) {
	return s.Get(ctx, id)
}
func (s *couponTestStore) Create(_ context.Context, command couponport.UpsertCommand, _ []int64, now time.Time) (couponport.Coupon, error) {
	id := s.nextID
	s.nextID++
	item := command.Coupon
	item.ID, item.Status, item.Currency = id, "draft", "CNY"
	item.CreatedBy, item.UpdatedBy, item.Version, item.CreatedAt, item.UpdatedAt = command.Actor, command.Actor, 1, now, now
	s.coupons[id] = item
	return item, nil
}
func (s *couponTestStore) Update(_ context.Context, command couponport.UpsertCommand, _ []int64, now time.Time) (couponport.Coupon, error) {
	s.updates++
	old, err := s.Get(context.Background(), command.ID)
	if err != nil {
		return couponport.Coupon{}, err
	}
	item := command.Coupon
	item.Status, item.Currency = old.Status, "CNY"
	item.CreatedBy, item.UpdatedBy, item.Version, item.CreatedAt, item.UpdatedAt = old.CreatedBy, command.Actor, old.Version+1, old.CreatedAt, now
	s.coupons[item.ID] = item
	return item, nil
}
func (s *couponTestStore) SetStatus(_ context.Context, id couponport.ID, status string, actor int64, now time.Time) (couponport.Coupon, error) {
	item, err := s.Get(context.Background(), id)
	if err != nil {
		return couponport.Coupon{}, err
	}
	item.Status, item.UpdatedBy, item.UpdatedAt, item.Version = status, actor, now, item.Version+1
	s.coupons[id] = item
	return item, nil
}
func couponTestReceiptKey(x Reservation) string {
	return x.Operation + "\x00" + x.ActorScope + "\x00" + fmt.Sprintf("%x", x.KeyDigest)
}
func (s *couponTestStore) Reserve(_ context.Context, x Reservation) (Receipt, bool, error) {
	key := couponTestReceiptKey(x)
	if row, ok := s.receipts[key]; ok {
		return row, false, nil
	}
	row := Receipt{ID: int64(len(s.receipts) + 1), Operation: x.Operation, ActorScope: x.ActorScope, KeyDigest: x.KeyDigest, PayloadDigest: x.PayloadDigest, State: "in_progress"}
	s.receipts[key] = row
	return row, true, nil
}
func (s *couponTestStore) Complete(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (Receipt, error) {
	for key, row := range s.receipts {
		if row.ID != id || row.State != "in_progress" {
			continue
		}
		row.State, row.ResultSnapshot = "completed", append(json.RawMessage(nil), snapshot...)
		s.receipts[key] = row
		return row, nil
	}
	return Receipt{}, ErrUnavailable
}
func (s *couponTestStore) DeleteDraft(_ context.Context, id couponport.ID) error {
	item, err := s.Get(context.Background(), id)
	if err != nil {
		return err
	}
	if item.Status != "draft" || item.IssuedCount != 0 {
		return ErrConflict
	}
	delete(s.coupons, id)
	return nil
}
func (s *couponTestStore) ListClaims(_ context.Context, id couponport.ID, limit, offset int32) ([]couponport.Claim, error) {
	items := make([]couponport.Claim, 0, len(s.claims))
	for _, item := range s.claims {
		if item.CouponID == int64(id) {
			items = append(items, item)
		}
	}
	start := int(offset)
	if start > len(items) {
		start = len(items)
	}
	end := start + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return append([]couponport.Claim(nil), items[start:end]...), nil
}
func (s *couponTestStore) CountClaims(_ context.Context, id couponport.ID) (int64, error) {
	items, _ := s.ListClaims(context.Background(), id, MaximumLimit, 0)
	return int64(len(items)), nil
}
func (s *couponTestStore) CountCustomerClaims(_ context.Context, id couponport.ID, customer int64) (int64, error) {
	var count int64
	for _, item := range s.claims {
		if item.CouponID == int64(id) && item.CustomerID == customer {
			count++
		}
	}
	return count, nil
}
func (s *couponTestStore) CreateClaim(_ context.Context, id couponport.ID, customer int64, number int32, ref string, now time.Time) (couponport.Claim, error) {
	item := couponport.Claim{ID: int64(len(s.claims) + 1), CouponID: int64(id), CustomerID: customer, ClaimNumber: number, ClaimRef: ref, Status: "claimed", ClaimedAt: now}
	s.claims = append(s.claims, item)
	return item, nil
}
func (s *couponTestStore) IncrementIssued(_ context.Context, id couponport.ID, now time.Time) error {
	item, err := s.Get(context.Background(), id)
	if err != nil {
		return err
	}
	if item.IssuedCount >= item.TotalIssueLimit {
		return ErrConflict
	}
	item.IssuedCount++
	item.UpdatedAt, item.Version = now, item.Version+1
	s.coupons[id] = item
	return nil
}
func (s *couponTestStore) ListAvailable(_ context.Context, target string, customer int64, now time.Time, limit int32) ([]couponport.Coupon, error) {
	items := []couponport.Coupon{}
	for _, item := range s.coupons {
		count, _ := s.CountCustomerClaims(context.Background(), item.ID, customer)
		if item.Status == "published" && item.IssuedCount < item.TotalIssueLimit && count < item.PerUserIssueLimit && !now.Before(item.ClaimStartsAt) && now.Before(item.ClaimEndsAt) {
			for _, ref := range item.TargetRefs {
				if ref == target {
					items = append(items, item)
				}
			}
		}
	}
	if len(items) > int(limit) {
		items = items[:limit]
	}
	return items, nil
}
func (s *couponTestStore) ResolvePaymentIdentitySession(_ context.Context, digest [32]byte, _ time.Time) (int64, error) {
	id, ok := s.payment[digest]
	if !ok {
		return 0, ErrNotFound
	}
	return id, nil
}
func (s *couponTestStore) ResolveSidebarGrant(_ context.Context, digest [32]byte, _ time.Time) (int64, error) {
	id, ok := s.sidebar[digest]
	if !ok {
		return 0, ErrNotFound
	}
	return id, nil
}
func (s *couponTestStore) ListSidebarClaims(_ context.Context, customer int64, limit int32) ([]couponport.SidebarCoupon, error) {
	items := []couponport.SidebarCoupon{}
	for i := len(s.claims) - 1; i >= 0 && len(items) < int(limit); i-- {
		claim := s.claims[i]
		if claim.CustomerID != customer {
			continue
		}
		coupon, err := s.Get(context.Background(), couponport.ID(claim.CouponID))
		if err != nil {
			return nil, err
		}
		items = append(items, couponport.SidebarCoupon{CouponID: coupon.ID, CouponName: coupon.Name, CouponStatus: coupon.Status, ClaimRef: claim.ClaimRef, ClaimedAt: claim.ClaimedAt})
	}
	return items, nil
}

func couponTestService(now time.Time, store *couponTestStore, events *couponTestEvents) *Service {
	service := NewService(couponTestUOW{}, store, couponTestProducts{}, events)
	service.now = func() time.Time { return now }
	return service
}
func couponTestItem(id couponport.ID, now time.Time) couponport.Coupon {
	days := int32(30)
	return couponport.Coupon{ID: id, Name: "满减券", DiscountAmountTotal: 100, Currency: "CNY", Status: "published", TotalIssueLimit: 2, PerUserIssueLimit: 1, ClaimStartsAt: now.Add(-time.Hour), ClaimEndsAt: now.Add(time.Hour), ValidityMode: couponport.ValidityRelativeDays, RelativeValidityDays: &days, Instructions: "", TargetRefs: []string{"standard_product:7"}, CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour)}
}

func TestCouponClaimStateMachineReplaysAndCaps(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	store, events := newCouponTestStore(couponTestItem(7, now)), &couponTestEvents{}
	service := couponTestService(now, store, events)
	command := couponport.ClaimCommand{CouponID: 7, CustomerID: 41, IdempotencyKey: "claim-key-0000001"}
	first, err := service.Claim(context.Background(), command)
	if err != nil || first.ClaimRef == "" {
		t.Fatalf("first claim=%#v err=%v", first, err)
	}
	if items, listErr := service.ListAvailable(context.Background(), "standard_product:7", 41); listErr != nil || len(items) != 0 {
		t.Fatalf("claimed customer availability=%#v err=%v", items, listErr)
	}
	if items, listErr := service.ListAvailable(context.Background(), "standard_product:7", 42); listErr != nil || len(items) != 1 {
		t.Fatalf("other customer availability=%#v err=%v", items, listErr)
	}
	replay, err := service.Claim(context.Background(), command)
	if err != nil || replay != first || len(store.claims) != 1 || len(events.rows) != 1 || store.coupons[7].IssuedCount != 1 {
		t.Fatalf("replay=%#v err=%v claims=%d events=%d issued=%d", replay, err, len(store.claims), len(events.rows), store.coupons[7].IssuedCount)
	}
	if _, err = service.Claim(context.Background(), couponport.ClaimCommand{CouponID: 7, CustomerID: 41, IdempotencyKey: "claim-key-0000002"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same customer cap error=%v", err)
	}
	if _, err = service.Claim(context.Background(), couponport.ClaimCommand{CouponID: 7, CustomerID: 42, IdempotencyKey: "claim-key-0000003"}); err != nil {
		t.Fatalf("second customer error=%v", err)
	}
	updated := store.coupons[7]
	updated.TotalIssueLimit = 3
	if _, err = service.Update(context.Background(), couponport.UpsertCommand{Coupon: updated, Actor: 1, IdempotencyKey: "claimed-limit-key1"}); err != nil {
		t.Fatalf("claimed quantity increase error=%v", err)
	}
	updated.Name = "篡改规则"
	if _, err = service.Update(context.Background(), couponport.UpsertCommand{Coupon: updated, Actor: 1, IdempotencyKey: "claimed-rule-key02"}); !errors.Is(err, ErrRulesFrozen) {
		t.Fatalf("claimed rule edit error=%v", err)
	}
	if _, err = service.Claim(context.Background(), couponport.ClaimCommand{CouponID: 7, CustomerID: 43, IdempotencyKey: "claim-key-0000004"}); err != nil {
		t.Fatalf("increased total claim error=%v", err)
	}
	if _, err = service.Claim(context.Background(), couponport.ClaimCommand{CouponID: 7, CustomerID: 44, IdempotencyKey: "claim-key-0000005"}); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("sold out error=%v", err)
	}
	if len(events.rows) != 4 || events.rows[0].Type != "coupon.claimed" || events.rows[0].CustomerID != 41 {
		t.Fatalf("events=%#v", events.rows)
	}
}

func TestUpdateDraftLocksAndRejectsPublishedOrClaimedRules(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	assertRejected := func(t *testing.T, service *Service, store *couponTestStore, events *couponTestEvents) {
		t.Helper()
		current := store.coupons[7]
		beforeUpdates, beforeEvents := store.updates, len(events.rows)
		_, err := service.UpdateDraft(context.Background(), couponport.UpsertCommand{
			Coupon:         current,
			Actor:          1,
			IdempotencyKey: "browser-draft-update-key",
		})
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("UpdateDraft error=%v", err)
		}
		if store.updates != beforeUpdates || len(events.rows) != beforeEvents {
			t.Fatalf("rejected draft update wrote update/events: updates=%d events=%d", store.updates, len(events.rows))
		}
		for _, receipt := range store.receipts {
			if receipt.Operation == "update" && receipt.State == "completed" {
				t.Fatalf("rejected draft update completed receipt=%#v", receipt)
			}
		}
	}
	t.Run("published row", func(t *testing.T) {
		item := couponTestItem(7, now)
		item.Status = "draft"
		store, events := newCouponTestStore(item), &couponTestEvents{}
		service := couponTestService(now, store, events)
		if _, err := service.Publish(context.Background(), 7, 1, "publish-before-browser-update"); err != nil {
			t.Fatalf("publish=%v", err)
		}
		assertRejected(t, service, store, events)
	})
	t.Run("claimed row", func(t *testing.T) {
		item := couponTestItem(7, now)
		item.Status = "draft"
		store, events := newCouponTestStore(item), &couponTestEvents{}
		service := couponTestService(now, store, events)
		if _, err := service.Publish(context.Background(), 7, 1, "publish-before-claim"); err != nil {
			t.Fatalf("publish=%v", err)
		}
		if _, err := service.Claim(context.Background(), couponport.ClaimCommand{CouponID: 7, CustomerID: 41, IdempotencyKey: "claim-before-browser-update"}); err != nil {
			t.Fatalf("claim=%v", err)
		}
		assertRejected(t, service, store, events)
	})
}

func TestCouponBoardMutationsUseReceiptReplayAndConflicts(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	first, second := couponTestItem(7, now), couponTestItem(8, now)
	first.Status, second.Status = "draft", "draft"
	store, events := newCouponTestStore(first, second), &couponTestEvents{}
	service := couponTestService(now, store, events)
	key := "archive-key-00001"
	archived, err := service.Archive(context.Background(), 7, 9, key)
	if err != nil || archived.Status != "archived" || len(events.rows) != 1 {
		t.Fatalf("archive=%#v err=%v events=%d", archived, err, len(events.rows))
	}
	if replay, replayErr := service.Archive(context.Background(), 7, 9, key); replayErr != nil || replay.ID != archived.ID || replay.Status != archived.Status || len(events.rows) != 1 {
		t.Fatalf("archive replay=%#v err=%v events=%d", replay, replayErr, len(events.rows))
	}
	if _, err = service.Archive(context.Background(), 8, 9, key); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-coupon same-key error=%v", err)
	}
	deleted, err := service.Delete(context.Background(), 8, 9, "delete-key-000001")
	if err != nil || deleted.Status != "deleted" {
		t.Fatalf("delete=%#v err=%v", deleted, err)
	}
	if replay, replayErr := service.Delete(context.Background(), 8, 9, "delete-key-000001"); replayErr != nil || replay.Status != "deleted" {
		t.Fatalf("delete replay=%#v err=%v", replay, replayErr)
	}
	copied, err := service.Copy(context.Background(), 7, 9, "copy-key-00000001")
	if err != nil || copied.ID == 7 || copied.Name != "满减券 副本" || copied.Status != "draft" || copied.AvailabilityStatus != "draft" || copied.IssuedCount != 0 || copied.CreatedBy != 9 || copied.UpdatedBy != 9 {
		t.Fatalf("copy=%#v err=%v", copied, err)
	}
	if replay, replayErr := service.Copy(context.Background(), 7, 9, "copy-key-00000001"); replayErr != nil || replay.ID != copied.ID || len(events.rows) != 3 || events.rows[2].Type != "coupon.copied" {
		t.Fatalf("copy replay=%#v err=%v events=%#v", replay, replayErr, events.rows)
	}
	if _, err = service.Copy(context.Background(), 8, 9, "copy-key-00000001"); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-coupon copy key error=%v", err)
	}
}

func TestHistoricalCouponsRejectCurrentMutations(t *testing.T) {
	for _, issued := range []int64{0, 1} {
		for _, operation := range []string{"update", "update_draft", "copy", "archive", "delete", "publish", "stop", "claim"} {
			t.Run(fmt.Sprintf("%s/issued_%d", operation, issued), func(t *testing.T) {
				now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
				item := couponTestItem(7, now)
				item.Status, item.HistoryOnly, item.IssuedCount = "archived", true, issued
				store, events := newCouponTestStore(item), &couponTestEvents{}
				service := couponTestService(now, store, events)
				command := couponport.UpsertCommand{Coupon: item, Actor: 1, IdempotencyKey: "history-mutation-test-key"}
				command.TotalIssueLimit++
				var err error
				switch operation {
				case "update":
					_, err = service.Update(context.Background(), command)
				case "update_draft":
					_, err = service.UpdateDraft(context.Background(), command)
				case "copy":
					_, err = service.Copy(context.Background(), 7, 1, command.IdempotencyKey)
				case "archive":
					_, err = service.Archive(context.Background(), 7, 1, command.IdempotencyKey)
				case "delete":
					_, err = service.Delete(context.Background(), 7, 1, command.IdempotencyKey)
				case "publish":
					_, err = service.Publish(context.Background(), 7, 1, command.IdempotencyKey)
				case "stop":
					_, err = service.Stop(context.Background(), 7, 1, command.IdempotencyKey)
				case "claim":
					_, err = service.Claim(context.Background(), couponport.ClaimCommand{CouponID: 7, CustomerID: 41, IdempotencyKey: command.IdempotencyKey})
				}
				if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrNotClaimable) {
					t.Fatalf("historical %s accepted: %v", operation, err)
				}
				if len(store.coupons) != 1 || !reflect.DeepEqual(store.coupons[7], item) || store.updates != 0 || len(store.claims) != 0 || len(events.rows) != 0 {
					t.Fatal("historical mutation wrote current business state")
				}
				for _, receipt := range store.receipts {
					if receipt.State == "completed" {
						t.Fatal("historical mutation produced a success receipt")
					}
				}
			})
		}
	}
}

func TestCouponOpaqueIdentityAndSidebarGrantAreSeparate(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	store, events := newCouponTestStore(couponTestItem(7, now)), &couponTestEvents{}
	service := couponTestService(now, store, events)
	payment := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	sidebar := base64.RawURLEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
	store.payment[sha256.Sum256([]byte(payment))] = 41
	store.sidebar[sha256.Sum256([]byte(sidebar))] = 42
	if got, err := service.ResolvePaymentIdentitySession(context.Background(), payment); err != nil || got != 41 {
		t.Fatalf("payment customer=%d err=%v", got, err)
	}
	if _, err := service.ResolvePaymentIdentitySession(context.Background(), sidebar); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("sidebar token accepted as payment err=%v", err)
	}
	if got, err := service.ResolveSidebarGrant(context.Background(), sidebar); err != nil || got != 42 {
		t.Fatalf("sidebar customer=%d err=%v", got, err)
	}
	if _, err := service.ResolveSidebarGrant(context.Background(), payment); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("payment token accepted as sidebar err=%v", err)
	}
	if _, err := service.ResolvePaymentIdentitySession(context.Background(), "customer:41"); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("forged identity token err=%v", err)
	}
}
