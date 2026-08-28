package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	DefaultLimit  int32 = 50
	MaximumLimit  int32 = 200
	MaximumOffset int32 = 1_000_000
)

var (
	ErrInvalidCoupon = errors.New("invalid coupon")
	ErrInvalidTarget = errors.New("invalid coupon target")
	ErrNotFound      = errors.New("coupon not found")
	ErrConflict      = errors.New("coupon command conflict")
	ErrRulesFrozen   = errors.New("claimed coupon rules are frozen")
	ErrUnavailable   = errors.New("coupon service unavailable")
	ErrNotClaimable  = errors.New("coupon is not claimable")
)

type Receipt struct {
	ID                           int64
	Operation, ActorScope, State string
	KeyDigest, PayloadDigest     [32]byte
	ResultSnapshot               json.RawMessage
}

type BoardStore interface {
	Store
	DeleteDraft(context.Context, couponport.ID) error
	ListClaims(context.Context, couponport.ID, int32, int32) ([]couponport.Claim, error)
	CountClaims(context.Context, couponport.ID) (int64, error)
	CountCustomerClaims(context.Context, couponport.ID, int64) (int64, error)
	CreateClaim(context.Context, couponport.ID, int64, int32, string, time.Time) (couponport.Claim, error)
	IncrementIssued(context.Context, couponport.ID, time.Time) error
	ListAvailable(context.Context, string, int64, time.Time, int32) ([]couponport.Coupon, error)
	ResolvePaymentIdentitySession(context.Context, [32]byte, time.Time) (int64, error)
	ResolveSidebarGrant(context.Context, [32]byte, time.Time) (int64, error)
	ListSidebarClaims(context.Context, int64, int32) ([]couponport.SidebarCoupon, error)
}
type Reservation struct {
	Operation, ActorScope    string
	KeyDigest, PayloadDigest [32]byte
	CreatedAt                time.Time
}

type Store interface {
	List(context.Context, int32, int32, string, string) ([]couponport.Coupon, error)
	Count(context.Context, string, string) (int64, error)
	Get(context.Context, couponport.ID) (couponport.Coupon, error)
	Lock(context.Context, couponport.ID) (couponport.Coupon, error)
	Create(context.Context, couponport.UpsertCommand, []int64, time.Time) (couponport.Coupon, error)
	Update(context.Context, couponport.UpsertCommand, []int64, time.Time) (couponport.Coupon, error)
	SetStatus(context.Context, couponport.ID, string, int64, time.Time) (couponport.Coupon, error)
	Reserve(context.Context, Reservation) (Receipt, bool, error)
	Complete(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
}

type ProductReader interface {
	Get(context.Context, productport.ID) (productport.Product, error)
}

type Service struct {
	uow      platformport.UnitOfWork
	store    Store
	products ProductReader
	events   eventport.Appender
	now      func() time.Time
}

func NewService(uow platformport.UnitOfWork, store Store, products ProductReader, events eventport.Appender) *Service {
	return &Service{uow: uow, store: store, products: products, events: events, now: time.Now}
}

func (s *Service) List(ctx context.Context, limit, offset int32, search, status string) (couponport.Page, error) {
	if !ready(s) {
		return couponport.Page{}, ErrUnavailable
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	search, status = strings.TrimSpace(search), strings.TrimSpace(status)
	if limit < 1 || limit > MaximumLimit || offset < 0 || offset > MaximumOffset || len(search) > 80 || len(status) > 32 || status != "" && status != "draft" && status != "published" && status != "stopped" && status != "archived" {
		return couponport.Page{}, ErrInvalidCoupon
	}
	page := couponport.Page{Limit: limit, Offset: offset}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		page.Items, err = s.store.List(tx, limit, offset, search, status)
		if err == nil {
			page.Total, err = s.store.Count(tx, search, status)
		}
		return err
	})
	if err != nil {
		return couponport.Page{}, classify(err)
	}
	if page.Total < 0 || len(page.Items) > int(limit) {
		return couponport.Page{}, ErrUnavailable
	}
	for i := range page.Items {
		if !validStored(page.Items[i]) {
			return couponport.Page{}, ErrUnavailable
		}
		page.Items[i] = withAvailability(page.Items[i], s.now().UTC())
	}
	return page, nil
}

func (s *Service) Get(ctx context.Context, id couponport.ID) (couponport.Coupon, error) {
	if !ready(s) || id < 1 {
		return couponport.Coupon{}, ErrNotFound
	}
	var result couponport.Coupon
	err := s.uow.Within(ctx, func(tx context.Context) error { var err error; result, err = s.store.Get(tx, id); return err })
	if err != nil {
		return couponport.Coupon{}, classify(err)
	}
	if !validStored(result) {
		return couponport.Coupon{}, ErrUnavailable
	}
	return withAvailability(result, s.now().UTC()), nil
}

func (s *Service) Create(ctx context.Context, input couponport.UpsertCommand) (couponport.Coupon, error) {
	input.ID = 0
	return s.mutate(ctx, "create", input, "", false)
}
func (s *Service) Update(ctx context.Context, input couponport.UpsertCommand) (couponport.Coupon, error) {
	if input.ID < 1 {
		return couponport.Coupon{}, ErrNotFound
	}
	return s.mutate(ctx, "update", input, "", false)
}

// UpdateDraft is the browser-admin mutation boundary. It deliberately keeps
// the broader internal Update semantics (including claimed-rule limits) out
// of the legacy PUT route: the locked row must still be an unissued draft.
func (s *Service) UpdateDraft(ctx context.Context, input couponport.UpsertCommand) (couponport.Coupon, error) {
	if input.ID < 1 {
		return couponport.Coupon{}, ErrNotFound
	}
	return s.mutate(ctx, "update", input, "", true)
}
func (s *Service) Publish(ctx context.Context, id couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	return s.mutate(ctx, "publish", couponport.UpsertCommand{Coupon: couponport.Coupon{ID: id}, Actor: actor, IdempotencyKey: key}, "published", false)
}
func (s *Service) Stop(ctx context.Context, id couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	return s.mutate(ctx, "stop", couponport.UpsertCommand{Coupon: couponport.Coupon{ID: id}, Actor: actor, IdempotencyKey: key}, "stopped", false)
}

func (s *Service) Archive(ctx context.Context, id couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	return s.boardMutation(ctx, "archive", id, actor, key, func(tx context.Context, now time.Time) (couponport.Coupon, bool, error) {
		old, e := s.store.Lock(tx, id)
		if e != nil {
			return couponport.Coupon{}, false, e
		}
		if old.HistoryOnly {
			return couponport.Coupon{}, false, ErrConflict
		}
		if old.Status == "archived" {
			return withAvailability(old, now), false, nil
		}
		if old.Status != "draft" && old.Status != "published" && old.Status != "stopped" {
			return couponport.Coupon{}, false, ErrConflict
		}
		item, e := s.store.SetStatus(tx, id, "archived", actor, now)
		return withAvailability(item, now), e == nil, e
	})
}

func (s *Service) Delete(ctx context.Context, id couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	b, ok := s.store.(BoardStore)
	if !ok {
		return couponport.Coupon{}, ErrUnavailable
	}
	return s.boardMutation(ctx, "delete", id, actor, key, func(tx context.Context, now time.Time) (couponport.Coupon, bool, error) {
		old, e := b.Lock(tx, id)
		if e != nil {
			return couponport.Coupon{}, false, e
		}
		if old.Status != "draft" || old.IssuedCount != 0 {
			return couponport.Coupon{}, false, ErrConflict
		}
		if e = b.DeleteDraft(tx, id); e != nil {
			return couponport.Coupon{}, false, e
		}
		result := withAvailability(old, now)
		result.Status = "deleted"
		result.AvailabilityStatus = "deleted"
		return result, true, nil
	})
}

func (s *Service) Copy(ctx context.Context, id couponport.ID, actor int64, key string) (couponport.Coupon, error) {
	return s.boardMutation(ctx, "copy", id, actor, key, func(tx context.Context, now time.Time) (couponport.Coupon, bool, error) {
		old, e := s.store.Lock(tx, id)
		if e != nil {
			return couponport.Coupon{}, false, e
		}
		if old.HistoryOnly {
			return couponport.Coupon{}, false, ErrConflict
		}
		command, ids, e := normalize(couponport.UpsertCommand{Coupon: couponport.Coupon{Name: copiedCouponName(old.Name), DiscountAmountTotal: old.DiscountAmountTotal, TotalIssueLimit: old.TotalIssueLimit, PerUserIssueLimit: old.PerUserIssueLimit, ClaimStartsAt: old.ClaimStartsAt, ClaimEndsAt: old.ClaimEndsAt, ValidityMode: old.ValidityMode, UseStartsAt: old.UseStartsAt, UseEndsAt: old.UseEndsAt, RelativeValidityDays: old.RelativeValidityDays, Instructions: old.Instructions, TargetRefs: old.TargetRefs}, Actor: actor, IdempotencyKey: key})
		if e != nil {
			return couponport.Coupon{}, false, e
		}
		result, e := s.store.Create(tx, command, ids, now)
		return withAvailability(result, now), e == nil, e
	})
}

func (s *Service) boardMutation(ctx context.Context, operation string, id couponport.ID, actor int64, key string, apply func(context.Context, time.Time) (couponport.Coupon, bool, error)) (couponport.Coupon, error) {
	if !ready(s) || id < 1 || actor < 1 || !validBoardKey(key) || apply == nil {
		return couponport.Coupon{}, ErrInvalidCoupon
	}
	now := s.now().UTC()
	payload, e := json.Marshal(struct {
		CouponID  couponport.ID `json:"coupon_id"`
		Operation string        `json:"operation"`
	}{CouponID: id, Operation: operation})
	if e != nil {
		return couponport.Coupon{}, ErrUnavailable
	}
	reservation := Reservation{Operation: operation, ActorScope: fmt.Sprintf("admin:%d", actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	var result couponport.Coupon
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.Reserve(tx, reservation)
		if e != nil {
			return e
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validStored(result) {
				return ErrUnavailable
			}
			return nil
		}
		var changed bool
		result, changed, e = apply(tx, now)
		if e != nil {
			return e
		}
		if !validStored(result) {
			return ErrUnavailable
		}
		if changed {
			if e = s.appendBoardEvent(tx, operation, result, actor, key, now); e != nil {
				return e
			}
		}
		snapshot, e := json.Marshal(result)
		if e != nil {
			return e
		}
		completed, e := s.store.Complete(tx, receipt.ID, snapshot, now)
		if e != nil || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return couponport.Coupon{}, classify(err)
	}
	return result, nil
}

func (s *Service) Claim(ctx context.Context, input couponport.ClaimCommand) (couponport.Claim, error) {
	b, ok := s.store.(BoardStore)
	if !ready(s) || !ok || input.CouponID < 1 || input.CustomerID < 1 || !validBoardKey(input.IdempotencyKey) {
		return couponport.Claim{}, ErrInvalidCoupon
	}
	now := s.now().UTC()
	var result couponport.Claim
	payload, _ := json.Marshal(struct {
		CouponID couponport.ID
		Customer int64
	}{input.CouponID, input.CustomerID})
	reservation := Reservation{Operation: "claim", ActorScope: fmt.Sprintf("customer:%d", input.CustomerID), KeyDigest: sha256.Sum256([]byte(input.IdempotencyKey)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.Reserve(tx, reservation)
		if e != nil {
			return e
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validClaim(result) {
				return ErrUnavailable
			}
			return nil
		}
		old, e := b.Lock(tx, input.CouponID)
		if e != nil {
			return e
		}
		if withAvailability(old, now).AvailabilityStatus != "active" {
			return ErrNotClaimable
		}
		count, e := b.CountCustomerClaims(tx, input.CouponID, input.CustomerID)
		if e != nil {
			return e
		}
		if count >= old.PerUserIssueLimit || old.IssuedCount >= old.TotalIssueLimit {
			return ErrConflict
		}
		key := sha256.Sum256([]byte("coupon-claim\x00" + input.IdempotencyKey + "\x00" + strconv.FormatInt(input.CustomerID, 10)))
		ref := "cp_" + hex.EncodeToString(key[:16])
		result, e = b.CreateClaim(tx, input.CouponID, input.CustomerID, int32(count+1), ref, now)
		if e != nil {
			return e
		}
		if e = b.IncrementIssued(tx, input.CouponID, now); e != nil {
			return e
		}
		eventPayload, e := json.Marshal(struct {
			CouponID couponport.ID `json:"coupon_id"`
			ClaimRef string        `json:"claim_ref"`
		}{input.CouponID, result.ClaimRef})
		if e != nil {
			return e
		}
		if _, e = s.events.Append(tx, eventport.Event{Type: eventport.EvCouponClaimed, CustomerID: eventport.CustomerID(input.CustomerID), Payload: eventPayload, OccurredAt: now, IdempotencyKey: "coupon.claim:" + hex.EncodeToString(key[:])}); e != nil {
			return e
		}
		snapshot, e := json.Marshal(result)
		if e != nil {
			return e
		}
		completed, e := s.store.Complete(tx, receipt.ID, snapshot, now)
		if e != nil || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return couponport.Claim{}, classify(err)
	}
	return result, nil
}

func validClaim(c couponport.Claim) bool {
	return c.ID > 0 && c.CouponID > 0 && c.CustomerID > 0 && c.ClaimNumber > 0 && strings.HasPrefix(c.ClaimRef, "cp_") && c.Status == "claimed" && !c.ClaimedAt.IsZero()
}

func (s *Service) ListClaims(ctx context.Context, id couponport.ID, limit, offset int32) (couponport.ClaimPage, error) {
	b, ok := s.store.(BoardStore)
	if !ready(s) || !ok || id < 1 || limit < 1 || limit > MaximumLimit || offset < 0 {
		return couponport.ClaimPage{}, ErrInvalidCoupon
	}
	p := couponport.ClaimPage{Limit: limit, Offset: offset}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		if _, e = s.store.Get(tx, id); e != nil {
			return e
		}
		p.Items, e = b.ListClaims(tx, id, limit, offset)
		if e == nil {
			p.Total, e = b.CountClaims(tx, id)
		}
		return e
	})
	if err != nil {
		return couponport.ClaimPage{}, classify(err)
	}
	return p, nil
}

func (s *Service) ListAvailable(ctx context.Context, target string, customerID int64) ([]couponport.Coupon, error) {
	b, ok := s.store.(BoardStore)
	target = strings.TrimSpace(target)
	if !ready(s) || !ok || target == "" || len(target) > 200 || customerID < 1 {
		return nil, ErrInvalidTarget
	}
	var items []couponport.Coupon
	now := s.now().UTC()
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		items, e = b.ListAvailable(tx, target, customerID, now, MaximumLimit)
		return e
	})
	if err != nil {
		return nil, classify(err)
	}
	for i := range items {
		items[i] = withAvailability(items[i], now)
	}
	return items, nil
}

func (s *Service) ResolvePaymentIdentitySession(ctx context.Context, token string) (int64, error) {
	b, ok := s.store.(BoardStore)
	if !ready(s) || !ok || !validOpaqueBrowserToken(token) {
		return 0, ErrNotClaimable
	}
	d := sha256.Sum256([]byte(token))
	var customerID int64
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		customerID, e = b.ResolvePaymentIdentitySession(tx, d, s.now().UTC())
		return e
	})
	if err != nil || customerID < 1 {
		return 0, ErrNotClaimable
	}
	return customerID, nil
}

func (s *Service) ResolveSidebarGrant(ctx context.Context, token string) (int64, error) {
	b, ok := s.store.(BoardStore)
	if !ready(s) || !ok || !validOpaqueBrowserToken(token) {
		return 0, ErrNotClaimable
	}
	digest := sha256.Sum256([]byte(token))
	var customerID int64
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		customerID, e = b.ResolveSidebarGrant(tx, digest, s.now().UTC())
		return e
	})
	if err != nil || customerID < 1 {
		return 0, ErrNotClaimable
	}
	return customerID, nil
}

func (s *Service) ListSidebarCoupons(ctx context.Context, customerID int64) ([]couponport.SidebarCoupon, error) {
	b, ok := s.store.(BoardStore)
	if !ready(s) || !ok || customerID < 1 {
		return nil, ErrInvalidCoupon
	}
	var result []couponport.SidebarCoupon
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		result, e = b.ListSidebarClaims(tx, customerID, 100)
		return e
	})
	if err != nil {
		return nil, classify(err)
	}
	for _, item := range result {
		if item.CouponID < 1 || strings.TrimSpace(item.CouponName) == "" || item.ClaimRef == "" || item.ClaimedAt.IsZero() {
			return nil, ErrUnavailable
		}
	}
	return result, nil
}

func validBoardKey(key string) bool {
	return len(key) >= 16 && len(key) <= 128 && strings.TrimSpace(key) == key
}

func validOpaqueBrowserToken(token string) bool {
	return len(token) == 43 && strings.TrimSpace(token) == token && validBase64URLToken(token)
}

func validBase64URLToken(token string) bool {
	for _, r := range token {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func copiedCouponName(name string) string {
	suffix := " 副本"
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 45-len([]rune(suffix)) {
		runes = runes[:45-len([]rune(suffix))]
	}
	return string(runes) + suffix
}
func (s *Service) appendBoardEvent(ctx context.Context, operation string, item couponport.Coupon, actor int64, key string, now time.Time) error {
	payload, e := json.Marshal(struct {
		CouponID couponport.ID `json:"coupon_id"`
		Actor    int64         `json:"actor"`
		Status   string        `json:"status"`
	}{item.ID, actor, item.Status})
	if e != nil {
		return e
	}
	d := sha256.Sum256([]byte("coupon." + operation + "\x00" + key))
	_, e = s.events.Append(ctx, eventport.Event{Type: eventType(operation), Payload: payload, OccurredAt: now, IdempotencyKey: "coupon." + operation + ":" + hex.EncodeToString(d[:])})
	return e
}

func (s *Service) mutate(ctx context.Context, operation string, input couponport.UpsertCommand, desired string, draftOnly bool) (couponport.Coupon, error) {
	minimumKeyLength := 1
	if operation == "create" || operation == "update" {
		minimumKeyLength = 16
	}
	if !ready(s) || input.Actor < 1 || len(input.IdempotencyKey) < minimumKeyLength || len(input.IdempotencyKey) > 128 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey {
		return couponport.Coupon{}, ErrInvalidCoupon
	}
	now := s.now().UTC()
	if now.IsZero() {
		return couponport.Coupon{}, ErrUnavailable
	}
	var command couponport.UpsertCommand
	var productIDs []int64
	var err error
	if operation == "create" || operation == "update" {
		command, productIDs, err = normalize(input)
		if err != nil {
			return couponport.Coupon{}, err
		}
	} else {
		command = input
	}
	digestRaw, _ := json.Marshal(struct {
		Operation string
		Command   couponport.UpsertCommand
		Desired   string
	}{operation, command, desired})
	reservation := Reservation{Operation: operation, ActorScope: fmt.Sprintf("admin:%d", input.Actor), KeyDigest: sha256.Sum256([]byte(input.IdempotencyKey)), PayloadDigest: sha256.Sum256(digestRaw), CreatedAt: now}
	var result couponport.Coupon
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.Reserve(tx, reservation)
		if e != nil {
			return e
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validStored(result) {
				return ErrUnavailable
			}
			return nil
		}
		changed := true
		switch operation {
		case "create":
			result, e = s.store.Create(tx, command, productIDs, now)
		case "update":
			var old couponport.Coupon
			old, e = s.store.Lock(tx, command.ID)
			if e == nil && old.HistoryOnly {
				e = ErrConflict
			}
			if e == nil && draftOnly {
				if old.Status != "draft" || old.IssuedCount != 0 {
					e = ErrConflict
				}
			} else if e == nil {
				if old.Status != "draft" && old.IssuedCount == 0 {
					e = ErrConflict
				}
				if e == nil && old.IssuedCount > 0 && !claimedUpdateAllowed(old, command.Coupon) {
					e = ErrRulesFrozen
				}
			}
			if e == nil {
				result, e = s.store.Update(tx, command, productIDs, now)
			}
		case "publish", "stop":
			var old couponport.Coupon
			old, e = s.store.Lock(tx, input.ID)
			if e == nil && old.Status == desired {
				result = old
				changed = false
			}
			if e == nil && changed && (operation == "publish" && old.Status != "draft" || operation == "stop" && old.Status != "published") {
				e = ErrConflict
			}
			if e == nil && changed && operation == "publish" {
				e = s.validateProducts(tx, old)
			}
			if e == nil && changed {
				result, e = s.store.SetStatus(tx, input.ID, desired, input.Actor, now)
			}
		default:
			e = ErrInvalidCoupon
		}
		if e != nil {
			return e
		}
		if !validStored(result) {
			return ErrUnavailable
		}
		result = withAvailability(result, now)
		if changed {
			payload, e := json.Marshal(struct {
				CouponID couponport.ID `json:"coupon_id"`
				Actor    int64         `json:"actor"`
				Status   string        `json:"status"`
			}{result.ID, input.Actor, result.Status})
			if e != nil {
				return e
			}
			d := sha256.Sum256([]byte(reservation.ActorScope + "\x00" + operation + "\x00" + input.IdempotencyKey))
			if _, e = s.events.Append(tx, eventport.Event{Type: eventType(operation), Payload: payload, OccurredAt: now, IdempotencyKey: "coupon." + operation + ":" + hex.EncodeToString(d[:])}); e != nil {
				return e
			}
		}
		snapshot, e := json.Marshal(result)
		if e != nil {
			return e
		}
		completed, e := s.store.Complete(tx, receipt.ID, snapshot, now)
		if e != nil || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return couponport.Coupon{}, classify(err)
	}
	return result, nil
}

func normalize(input couponport.UpsertCommand) (couponport.UpsertCommand, []int64, error) {
	c := &input.Coupon
	c.Name = strings.TrimSpace(c.Name)
	c.Instructions = strings.TrimSpace(c.Instructions)
	c.ClaimStartsAt = canonicalTime(c.ClaimStartsAt)
	c.ClaimEndsAt = canonicalTime(c.ClaimEndsAt)
	if c.UseStartsAt != nil {
		value := canonicalTime(*c.UseStartsAt)
		c.UseStartsAt = &value
	}
	if c.UseEndsAt != nil {
		value := canonicalTime(*c.UseEndsAt)
		c.UseEndsAt = &value
	}
	c.Currency = "CNY"
	c.Status = "draft"
	if c.Name == "" || len(c.Name) > 45 || c.DiscountAmountTotal < 1 || c.TotalIssueLimit < 1 || c.PerUserIssueLimit < 1 || c.PerUserIssueLimit > c.TotalIssueLimit || !c.ClaimEndsAt.After(c.ClaimStartsAt) || len(c.Instructions) > 200 || len(c.TargetRefs) < 1 || len(c.TargetRefs) > 100 {
		return couponport.UpsertCommand{}, nil, ErrInvalidCoupon
	}
	if c.ValidityMode == couponport.ValidityFixedRange {
		if c.UseStartsAt == nil || c.UseEndsAt == nil || !c.UseEndsAt.After(*c.UseStartsAt) || c.RelativeValidityDays != nil {
			return couponport.UpsertCommand{}, nil, ErrInvalidCoupon
		}
	} else if c.ValidityMode == couponport.ValidityRelativeDays {
		if c.UseStartsAt != nil || c.UseEndsAt != nil || c.RelativeValidityDays == nil || *c.RelativeValidityDays < 1 {
			return couponport.UpsertCommand{}, nil, ErrInvalidCoupon
		}
	} else {
		return couponport.UpsertCommand{}, nil, ErrInvalidCoupon
	}
	seen := map[string]bool{}
	ids := make([]int64, len(c.TargetRefs))
	for i, ref := range c.TargetRefs {
		if seen[ref] {
			return couponport.UpsertCommand{}, nil, ErrInvalidTarget
		}
		seen[ref] = true
		parts := strings.Split(ref, ":")
		if len(parts) != 2 || parts[0] != "standard_product" {
			return couponport.UpsertCommand{}, nil, ErrInvalidTarget
		}
		id, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 || parts[1] != strconv.FormatInt(id, 10) {
			return couponport.UpsertCommand{}, nil, ErrInvalidTarget
		}
		ids[i] = id
	}
	return input, ids, nil
}

func canonicalTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func (s *Service) validateProducts(ctx context.Context, c couponport.Coupon) error {
	seen := map[int64]bool{}
	for _, ref := range c.TargetRefs {
		parts := strings.Split(ref, ":")
		if len(parts) != 2 {
			return ErrInvalidTarget
		}
		id, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 || seen[id] {
			return ErrInvalidTarget
		}
		seen[id] = true
		p, e := s.products.Get(ctx, productport.ID(id))
		if e != nil || p.ID != productport.ID(id) || p.Currency != "CNY" || p.PriceMinor <= c.DiscountAmountTotal {
			return ErrInvalidTarget
		}
	}
	return nil
}
func claimedUpdateAllowed(old, next couponport.Coupon) bool {
	return next.TotalIssueLimit >= old.TotalIssueLimit && old.Name == next.Name && old.DiscountAmountTotal == next.DiscountAmountTotal && old.PerUserIssueLimit == next.PerUserIssueLimit && old.ClaimStartsAt.Equal(next.ClaimStartsAt) && old.ClaimEndsAt.Equal(next.ClaimEndsAt) && old.ValidityMode == next.ValidityMode && sameTime(old.UseStartsAt, next.UseStartsAt) && sameTime(old.UseEndsAt, next.UseEndsAt) && reflect.DeepEqual(old.RelativeValidityDays, next.RelativeValidityDays) && old.Instructions == next.Instructions && reflect.DeepEqual(old.TargetRefs, next.TargetRefs)
}
func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}
func validStored(c couponport.Coupon) bool {
	if c.ID < 1 || c.CreatedBy < 1 || c.UpdatedBy < 1 || c.Version < 1 || c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		return false
	}
	_, _, e := normalize(couponport.UpsertCommand{Coupon: c, Actor: c.UpdatedBy, IdempotencyKey: strings.Repeat("v", 16)})
	return e == nil
}
func withAvailability(c couponport.Coupon, now time.Time) couponport.Coupon {
	switch {
	case c.Status == "published" && c.IssuedCount >= c.TotalIssueLimit:
		c.AvailabilityStatus = "sold_out"
	case c.Status == "published" && now.Before(c.ClaimStartsAt):
		c.AvailabilityStatus = "scheduled"
	case c.Status == "published" && !now.Before(c.ClaimEndsAt):
		c.AvailabilityStatus = "ended"
	case c.Status == "published":
		c.AvailabilityStatus = "active"
	default:
		c.AvailabilityStatus = c.Status
	}
	return c
}
func ready(s *Service) bool {
	return s != nil && s.uow != nil && s.store != nil && s.products != nil && s.events != nil
}
func receiptMatches(r Receipt, x Reservation) bool {
	return r.ID > 0 && r.Operation == x.Operation && r.ActorScope == x.ActorScope && subtle.ConstantTimeCompare(r.KeyDigest[:], x.KeyDigest[:]) == 1
}
func jsonEquivalent(a, b []byte) bool {
	var x, y any
	da := json.NewDecoder(strings.NewReader(string(a)))
	db := json.NewDecoder(strings.NewReader(string(b)))
	da.UseNumber()
	db.UseNumber()
	return da.Decode(&x) == nil && db.Decode(&y) == nil && reflect.DeepEqual(x, y)
}
func classify(e error) error {
	switch {
	case errors.Is(e, ErrInvalidCoupon), errors.Is(e, ErrInvalidTarget), errors.Is(e, ErrNotFound), errors.Is(e, ErrConflict), errors.Is(e, ErrRulesFrozen), errors.Is(e, ErrNotClaimable):
		return e
	default:
		return ErrUnavailable
	}
}
func eventType(operation string) string {
	switch operation {
	case "create":
		return eventport.EvCouponCreated
	case "update":
		return eventport.EvCouponUpdated
	case "publish":
		return eventport.EvCouponPublished
	case "stop":
		return eventport.EvCouponStopped
	case "archive":
		return eventport.EvCouponArchived
	case "delete":
		return eventport.EvCouponDeleted
	case "copy":
		return eventport.EvCouponCopied
	default:
		return ""
	}
}
