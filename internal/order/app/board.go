package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidBoardCommand = errors.New("invalid order board command")
	ErrBoardConflict       = errors.New("order board conflict")
	ErrBoardUnavailable    = errors.New("order board unavailable")
)

type BoardReceipt struct {
	ID                           int64
	Operation, ActorScope, State string
	KeyDigest, PayloadDigest     [32]byte
	ResultSnapshot               json.RawMessage
}

type BoardReservation struct {
	Operation, ActorScope    string
	KeyDigest, PayloadDigest [32]byte
	CreatedAt                time.Time
}

// BoardStore is Order-owned. It deliberately has no provider method: runtime
// records the local request and the external gate, but cannot call or retry a
// payment provider without a separately approved provider slice.
type BoardStore interface {
	ListBoardOrders(context.Context, orderport.BoardFilter) ([]orderport.Record, error)
	CountBoardOrders(context.Context, orderport.BoardFilter) (int64, error)
	GetBoardOrder(context.Context, string, string) (orderport.Record, error)
	LockBoardOrder(context.Context, string, string) (orderport.Record, error)
	CountActiveRefundAmount(context.Context, orderport.ID) (int64, error)
	ReserveBoardReceipt(context.Context, BoardReservation) (BoardReceipt, bool, error)
	CompleteBoardReceipt(context.Context, int64, json.RawMessage, time.Time) (BoardReceipt, error)
	CreateExportJob(context.Context, orderport.ExportJob) (orderport.ExportJob, error)
	GetExportJob(context.Context, string) (orderport.ExportJob, error)
	CreateExternalEffect(context.Context, orderport.ExternalEffect) (orderport.ExternalEffect, error)
	GetExternalEffect(context.Context, int64, bool) (orderport.ExternalEffect, error)
	ListExternalEffects(context.Context, orderport.ID) ([]orderport.ExternalEffect, int64, error)
	RequestExternalEffectReview(context.Context, int64, time.Time) (orderport.ExternalEffect, error)
	CreateRefund(context.Context, orderport.Refund) (orderport.Refund, error)
	ListRefunds(context.Context, orderport.RefundFilter) ([]orderport.Refund, int64, error)
}

type BoardService struct {
	uow    platformport.UnitOfWork
	store  BoardStore
	events eventport.Appender
	now    func() time.Time
}

func NewBoardService(uow platformport.UnitOfWork, store BoardStore, events eventport.Appender) *BoardService {
	return &BoardService{uow: uow, store: store, events: events, now: time.Now}
}

func (s *BoardService) ListOrders(ctx context.Context, filter orderport.BoardFilter) (orderport.Page, error) {
	filter, err := normalizeBoardFilter(ctx, filter)
	if err != nil || !boardReady(s) {
		return orderport.Page{}, boardClassify(err)
	}
	page := orderport.Page{Limit: filter.Limit, Items: []orderport.Item{}}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		rows, e := s.store.ListBoardOrders(tx, filter)
		if e != nil {
			return e
		}
		page.Total, e = s.store.CountBoardOrders(tx, filter)
		if e != nil {
			return e
		}
		if page.Total < 0 || len(rows) > int(filter.Limit) || int64(filter.Offset)+int64(len(rows)) > page.Total {
			return ErrBoardUnavailable
		}
		for _, row := range rows {
			if !validRecord(row) {
				return ErrBoardUnavailable
			}
			page.Items = append(page.Items, boardItem(row))
		}
		return nil
	})
	if err != nil {
		return orderport.Page{}, boardClassify(err)
	}
	page.HasMore = int64(filter.Offset)+int64(len(page.Items)) < page.Total
	return page, nil
}

func (s *BoardService) GetOrder(ctx context.Context, provider, reference string) (orderport.Detail, error) {
	provider, reference, err := normalizeReference(ctx, provider, reference)
	if err != nil || !boardReady(s) {
		return orderport.Detail{}, boardClassify(err)
	}
	var result orderport.Detail
	err = s.uow.Within(ctx, func(tx context.Context) error {
		row, e := s.store.GetBoardOrder(tx, provider, reference)
		if e != nil {
			return e
		}
		if !validRecord(row) {
			return ErrBoardUnavailable
		}
		active, e := s.store.CountActiveRefundAmount(tx, row.ID)
		if e != nil || active < 0 || active > row.AmountMinor {
			return ErrBoardUnavailable
		}
		result = orderport.Detail{Item: boardItem(row), ID: row.ID, RefundableAmountMinor: row.AmountMinor - active}
		return nil
	})
	if err != nil {
		return orderport.Detail{}, boardClassify(err)
	}
	return result, nil
}

func (s *BoardService) ListRefunds(ctx context.Context, filter orderport.RefundFilter) (orderport.RefundPage, error) {
	filter, err := normalizeRefundFilter(ctx, filter)
	if err != nil || !boardReady(s) {
		return orderport.RefundPage{}, boardClassify(err)
	}
	page := orderport.RefundPage{Limit: filter.Limit, Items: []orderport.Refund{}}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		page.Items, page.Total, e = s.store.ListRefunds(tx, filter)
		if e != nil || page.Total < 0 || len(page.Items) > int(filter.Limit) || int64(filter.Offset)+int64(len(page.Items)) > page.Total {
			return ErrBoardUnavailable
		}
		for _, item := range page.Items {
			if !validRefund(item) {
				return ErrBoardUnavailable
			}
		}
		return nil
	})
	if err != nil {
		return orderport.RefundPage{}, boardClassify(err)
	}
	page.HasMore = int64(filter.Offset)+int64(len(page.Items)) < page.Total
	return page, nil
}

func (s *BoardService) CreateExport(ctx context.Context, command orderport.ExportCommand) (orderport.ExportJob, error) {
	if !boardReady(s) || !validExport(command) {
		return orderport.ExportJob{}, ErrInvalidBoardCommand
	}
	now := s.now().UTC()
	payload, _ := json.Marshal(struct {
		Resource, Format string
		Filters          json.RawMessage
	}{command.Resource, command.Format, command.Filters})
	reservation := BoardReservation{Operation: "export", ActorScope: actorScope(command.Actor), KeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	var result orderport.ExportJob
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.ReserveBoardReceipt(tx, reservation)
		if e != nil {
			return e
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrBoardConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validExportJob(result) {
				return ErrBoardUnavailable
			}
			return nil
		}
		content, e := s.exportCSV(tx, command)
		if e != nil {
			return e
		}
		result, e = s.store.CreateExportJob(tx, orderport.ExportJob{JobID: fmt.Sprintf("exp_%016x", receipt.ID), Resource: command.Resource, Format: command.Format, Status: "completed", Operator: command.Actor, ContentText: content, CreatedAt: now})
		if e != nil || !validExportJob(result) {
			return ErrBoardUnavailable
		}
		if e = s.append(tx, eventport.EvOrderExportCreated, reservation.KeyDigest, map[string]any{"job_id": result.JobID, "resource": result.Resource}); e != nil {
			return e
		}
		return s.completeExport(tx, receipt.ID, result, now)
	})
	if err != nil {
		return orderport.ExportJob{}, boardClassify(err)
	}
	return result, nil
}

func (s *BoardService) GetExport(ctx context.Context, jobID string) (orderport.ExportJob, error) {
	if !boardReady(s) || !validText(jobID, 68) || !strings.HasPrefix(jobID, "exp_") {
		return orderport.ExportJob{}, ErrInvalidBoardCommand
	}
	var result orderport.ExportJob
	err := s.uow.Within(ctx, func(tx context.Context) error { var e error; result, e = s.store.GetExportJob(tx, jobID); return e })
	if err != nil || !validExportJob(result) {
		return orderport.ExportJob{}, boardClassify(err)
	}
	return result, nil
}

func (s *BoardService) RequestRefund(ctx context.Context, command orderport.RefundCommand) (orderport.Refund, error) {
	command, err := normalizeRefundCommand(ctx, command)
	if err != nil || !boardReady(s) {
		return orderport.Refund{}, boardClassify(err)
	}
	now := s.now().UTC()
	payload, _ := json.Marshal(command)
	reservation := BoardReservation{Operation: "refund", ActorScope: actorScope(command.Actor), KeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	var result orderport.Refund
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.ReserveBoardReceipt(tx, reservation)
		if e != nil {
			return e
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrBoardConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validRefund(result) {
				return ErrBoardUnavailable
			}
			return nil
		}
		order, e := s.store.LockBoardOrder(tx, command.Provider, command.OrderReference)
		if e != nil {
			return e
		}
		if !validRecord(order) || order.Provider != command.Provider || (order.Status != "paid" && order.Status != "partially_refunded") || order.PlatformTransactionNo != command.TransactionIDConfirmation {
			return ErrInvalidBoardCommand
		}
		active, e := s.store.CountActiveRefundAmount(tx, order.ID)
		if e != nil || active < 0 || active > order.AmountMinor || command.RefundAmountTotal > order.AmountMinor-active {
			return ErrBoardConflict
		}
		effect, e := s.store.CreateExternalEffect(tx, orderport.ExternalEffect{OrderID: order.ID, Provider: order.Provider, EffectKind: "refund", State: "pending_external_gate", AutoRetryAllowed: false, CreatedAt: now, UpdatedAt: now})
		if e != nil || !validEffect(effect) {
			return ErrBoardUnavailable
		}
		ref := hex.EncodeToString(reservation.KeyDigest[:12])
		result, e = s.store.CreateRefund(tx, orderport.Refund{OrderID: order.ID, OrderNo: order.MerchantOrderNo, TransactionID: order.PlatformTransactionNo, Provider: order.Provider, RefundID: "rfd_" + ref, OutRefundNo: "rfd_" + ref, RefundAmountTotal: command.RefundAmountTotal, Currency: order.Currency, Reason: command.Reason, Status: "pending_external_gate", ExternalEffectID: effect.ID, ExternalEffectState: effect.State, AutoRetryAllowed: false, CreatedAt: now})
		if e != nil || !validRefund(result) {
			return ErrBoardUnavailable
		}
		if e = s.append(tx, eventport.EvOrderRefundRequested, reservation.KeyDigest, map[string]any{"refund_id": result.RefundID, "order_no": result.OrderNo, "external_effect_id": effect.ID, "external_effect_state": effect.State, "real_refund_executed": false}); e != nil {
			return e
		}
		return s.completeRefund(tx, receipt.ID, result, now)
	})
	if err != nil {
		return orderport.Refund{}, boardClassify(err)
	}
	return result, nil
}

func (s *BoardService) ListExternalEffects(ctx context.Context, provider, reference string) (orderport.ExternalEffectPage, error) {
	provider, reference, err := normalizeReference(ctx, provider, reference)
	if err != nil || !boardReady(s) {
		return orderport.ExternalEffectPage{}, boardClassify(err)
	}
	var result orderport.ExternalEffectPage
	err = s.uow.Within(ctx, func(tx context.Context) error {
		order, e := s.store.GetBoardOrder(tx, provider, reference)
		if e != nil {
			return e
		}
		if !validRecord(order) {
			return ErrNotFound
		}
		result.Items, result.Total, e = s.store.ListExternalEffects(tx, order.ID)
		if e != nil || result.Total < 0 || int64(len(result.Items)) != result.Total {
			return ErrBoardUnavailable
		}
		for _, item := range result.Items {
			if !validEffect(item) || item.OrderID != order.ID {
				return ErrBoardUnavailable
			}
		}
		return nil
	})
	if err != nil {
		return orderport.ExternalEffectPage{}, boardClassify(err)
	}
	return result, nil
}

func (s *BoardService) RequestExternalEffectRetry(ctx context.Context, effectID, actor int64, key string) (orderport.ExternalEffect, error) {
	if !boardReady(s) || effectID < 1 || actor < 1 || !validBoardKey(key) {
		return orderport.ExternalEffect{}, ErrInvalidBoardCommand
	}
	now := s.now().UTC()
	payload, _ := json.Marshal(struct {
		EffectID int64 `json:"effect_id"`
	}{effectID})
	reservation := BoardReservation{Operation: "external_effect_retry", ActorScope: actorScope(actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	var result orderport.ExternalEffect
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.ReserveBoardReceipt(tx, reservation)
		if e != nil {
			return e
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrBoardConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validEffect(result) {
				return ErrBoardUnavailable
			}
			return nil
		}
		result, e = s.store.GetExternalEffect(tx, effectID, true)
		if e != nil {
			return e
		}
		// outcome_unknown can only be resolved by provider evidence; it is never
		// retried automatically or by this local compatibility endpoint.
		if !validEffect(result) || result.State == "outcome_unknown" {
			return ErrBoardConflict
		}
		result, e = s.store.RequestExternalEffectReview(tx, effectID, now)
		if e != nil || !validEffect(result) {
			return ErrBoardUnavailable
		}
		if e = s.append(tx, eventport.EvOrderEffectRetryRequested, reservation.KeyDigest, map[string]any{"external_effect_id": result.ID, "state": result.State, "provider_call_executed": false}); e != nil {
			return e
		}
		snapshot, e := json.Marshal(result)
		if e != nil {
			return e
		}
		_, e = s.store.CompleteBoardReceipt(tx, receipt.ID, snapshot, now)
		return e
	})
	if err != nil {
		return orderport.ExternalEffect{}, boardClassify(err)
	}
	return result, nil
}

func (s *BoardService) completeExport(ctx context.Context, id int64, value orderport.ExportJob, now time.Time) error {
	snapshot, err := json.Marshal(value)
	if err != nil {
		return err
	}
	receipt, err := s.store.CompleteBoardReceipt(ctx, id, snapshot, now)
	if err != nil || receipt.State != "completed" || !jsonEquivalent(receipt.ResultSnapshot, snapshot) {
		return ErrBoardUnavailable
	}
	return nil
}
func (s *BoardService) completeRefund(ctx context.Context, id int64, value orderport.Refund, now time.Time) error {
	snapshot, err := json.Marshal(value)
	if err != nil {
		return err
	}
	receipt, err := s.store.CompleteBoardReceipt(ctx, id, snapshot, now)
	if err != nil || receipt.State != "completed" || !jsonEquivalent(receipt.ResultSnapshot, snapshot) {
		return ErrBoardUnavailable
	}
	return nil
}
func (s *BoardService) append(ctx context.Context, typ string, key [32]byte, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.events.Append(ctx, eventport.Event{Type: typ, Payload: encoded, OccurredAt: s.now().UTC(), IdempotencyKey: "order.board:" + typ + ":" + hex.EncodeToString(key[:])})
	return err
}
func (s *BoardService) exportCSV(ctx context.Context, command orderport.ExportCommand) (string, error) {
	// The frozen export mapping proves resource and csv, but not a filter DTO.
	// Reject an unproven filter rather than silently accepting or reinterpreting it.
	if len(command.Filters) > 0 && string(command.Filters) != "{}" {
		return "", ErrInvalidBoardCommand
	}
	const maximumBoardExportRecords = int64(MaximumOffset) + int64(MaximumLimit)
	if command.Resource == "refunds" {
		filter, err := normalizeRefundFilter(ctx, orderport.RefundFilter{Limit: MaximumLimit})
		if err != nil {
			return "", err
		}
		refunds := make([]orderport.Refund, 0)
		var total int64
		for offset := int32(0); ; offset += MaximumLimit {
			filter.Offset = offset
			page, count, pageErr := s.store.ListRefunds(ctx, filter)
			if pageErr != nil || count < 0 || count > maximumBoardExportRecords {
				if pageErr != nil {
					return "", pageErr
				}
				return "", ErrInvalidBoardCommand
			}
			total = count
			refunds = append(refunds, page...)
			if int64(len(refunds)) >= total {
				break
			}
			if len(page) == 0 {
				return "", ErrBoardUnavailable
			}
		}
		return csvText([][]string{{"refund_id", "out_refund_no", "order_no", "transaction_id", "amount", "currency", "status", "provider"}}, func(write func([]string) error) error {
			for _, item := range refunds {
				if err := write([]string{item.RefundID, item.OutRefundNo, item.OrderNo, item.TransactionID, fmt.Sprint(item.RefundAmountTotal), item.Currency, item.Status, item.Provider}); err != nil {
					return err
				}
			}
			return nil
		})
	}
	filter, err := normalizeBoardFilter(ctx, orderport.BoardFilter{Limit: MaximumLimit})
	if err != nil {
		return "", err
	}
	total, err := s.store.CountBoardOrders(ctx, filter)
	if err != nil || total < 0 || total > maximumBoardExportRecords {
		if err != nil {
			return "", err
		}
		return "", ErrInvalidBoardCommand
	}
	orders := make([]orderport.Record, 0, total)
	for offset := int32(0); int64(offset) < total; offset += MaximumLimit {
		filter.Offset = offset
		page, pageErr := s.store.ListBoardOrders(ctx, filter)
		if pageErr != nil {
			return "", pageErr
		}
		orders = append(orders, page...)
		if int64(len(orders)) >= total {
			break
		}
		if len(page) == 0 {
			return "", ErrBoardUnavailable
		}
	}
	return csvText([][]string{{"order_no", "transaction_id", "provider", "product_code", "amount", "currency", "status", "created_at"}}, func(write func([]string) error) error {
		for _, item := range orders {
			if !validRecord(item) {
				return ErrBoardUnavailable
			}
			if err := write([]string{item.MerchantOrderNo, item.PlatformTransactionNo, item.Provider, item.ProductCode, fmt.Sprint(item.AmountMinor), item.Currency, item.Status, item.CreatedAt.UTC().Format(time.RFC3339)}); err != nil {
				return err
			}
		}
		return nil
	})
}
func csvText(headers [][]string, rows func(func([]string) error) error) (string, error) {
	var out strings.Builder
	writer := csv.NewWriter(&out)
	for _, row := range headers {
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}
	if err := rows(writer.Write); err != nil {
		return "", err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return out.String(), nil
}
func boardItem(row orderport.Record) orderport.Item {
	item := orderport.Item{CreatedAt: row.CreatedAt, MerchantOrderNo: row.MerchantOrderNo, OutTradeNo: row.MerchantOrderNo, OrderNo: row.MerchantOrderNo, PlatformTransactionNo: row.PlatformTransactionNo, TransactionID: row.PlatformTransactionNo, PayerName: row.PayerNameSnapshot, Mobile: row.MobileSnapshot, ProductCode: row.ProductCode, ProductName: row.ProductNameSnapshot, AmountYuan: fmt.Sprintf("%d.%02d", row.AmountMinor/100, row.AmountMinor%100), Currency: row.Currency, Status: row.Status, StatusLabel: row.StatusLabel, Provider: row.Provider, ProviderLabel: row.ProviderLabel, DetailURL: row.DetailURL}
	switch row.IdentityKind {
	case "userid":
		item.UserID = row.IdentityValue
	case "external_userid":
		item.ExternalUserID = row.IdentityValue
	case "unionid":
		item.UnionID = row.IdentityValue
	}
	return item
}
func normalizeBoardFilter(ctx context.Context, filter orderport.BoardFilter) (orderport.BoardFilter, error) {
	if ctx == nil || ctx.Err() != nil {
		return orderport.BoardFilter{}, ErrInvalidBoardCommand
	}
	filter.Provider = strings.ToLower(strings.TrimSpace(filter.Provider))
	if filter.Provider == "" {
		filter.Provider = "all"
	}
	if filter.Provider != "all" && filter.Provider != "wechat" && filter.Provider != "alipay" && filter.Provider != "wechat_shop" {
		return orderport.BoardFilter{}, ErrInvalidBoardCommand
	}
	for _, value := range []*string{&filter.Status, &filter.ProductCode, &filter.Mobile, &filter.Identity, &filter.TransactionID, &filter.OrderNo} {
		*value = strings.TrimSpace(*value)
		if !validText(*value, 200) {
			return orderport.BoardFilter{}, ErrInvalidBoardCommand
		}
	}
	if filter.Limit == 0 {
		filter.Limit = DefaultLimit
	}
	if filter.Limit < 1 || filter.Limit > MaximumLimit || filter.Offset < 0 || filter.Offset > MaximumOffset || invalidTime(filter.CreatedFrom) || invalidTime(filter.CreatedTo) || filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedFrom.After(*filter.CreatedTo) {
		return orderport.BoardFilter{}, ErrInvalidBoardCommand
	}
	filter.CreatedFrom, filter.CreatedTo = utcTime(filter.CreatedFrom), utcTime(filter.CreatedTo)
	return filter, nil
}
func normalizeRefundFilter(ctx context.Context, filter orderport.RefundFilter) (orderport.RefundFilter, error) {
	if ctx == nil || ctx.Err() != nil {
		return orderport.RefundFilter{}, ErrInvalidBoardCommand
	}
	filter.Provider = strings.ToLower(strings.TrimSpace(filter.Provider))
	if filter.Provider == "" {
		filter.Provider = "all"
	}
	if filter.Provider != "all" && filter.Provider != "wechat" && filter.Provider != "alipay" && filter.Provider != "wechat_shop" {
		return orderport.RefundFilter{}, ErrInvalidBoardCommand
	}
	for _, value := range []*string{&filter.OrderNo, &filter.TransactionID, &filter.RefundID, &filter.OutRefundNo, &filter.Status} {
		*value = strings.TrimSpace(*value)
		if !validText(*value, 200) {
			return orderport.RefundFilter{}, ErrInvalidBoardCommand
		}
	}
	if filter.Limit == 0 {
		filter.Limit = DefaultLimit
	}
	if filter.Limit < 1 || filter.Limit > MaximumLimit || filter.Offset < 0 || filter.Offset > MaximumOffset || invalidTime(filter.CreatedFrom) || invalidTime(filter.CreatedTo) || filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedFrom.After(*filter.CreatedTo) {
		return orderport.RefundFilter{}, ErrInvalidBoardCommand
	}
	filter.CreatedFrom, filter.CreatedTo = utcTime(filter.CreatedFrom), utcTime(filter.CreatedTo)
	return filter, nil
}
func normalizeReference(ctx context.Context, provider, reference string) (string, string, error) {
	if ctx == nil || ctx.Err() != nil {
		return "", "", ErrInvalidBoardCommand
	}
	provider, reference = strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(reference)
	if provider == "" {
		provider = "auto"
	}
	if provider != "auto" && provider != "wechat" && provider != "alipay" && provider != "wechat_shop" || !validText(reference, 200) || reference == "" {
		return "", "", ErrInvalidBoardCommand
	}
	return provider, reference, nil
}
func normalizeRefundCommand(ctx context.Context, command orderport.RefundCommand) (orderport.RefundCommand, error) {
	provider, reference, err := normalizeReference(ctx, command.Provider, command.OrderReference)
	if err != nil || provider == "auto" {
		return orderport.RefundCommand{}, ErrInvalidBoardCommand
	}
	command.Provider, command.OrderReference = provider, reference
	command.TransactionIDConfirmation, command.Reason, command.IdempotencyKey = strings.TrimSpace(command.TransactionIDConfirmation), strings.TrimSpace(command.Reason), strings.TrimSpace(command.IdempotencyKey)
	if command.Actor < 1 || command.RefundAmountTotal < 1 || !command.Checked || !validText(command.TransactionIDConfirmation, 200) || command.TransactionIDConfirmation == "" || !validText(command.Reason, 500) || command.Reason == "" || !validBoardKey(command.IdempotencyKey) {
		return orderport.RefundCommand{}, ErrInvalidBoardCommand
	}
	return command, nil
}
func validExport(command orderport.ExportCommand) bool {
	return command.Actor > 0 && (command.Resource == "orders" || command.Resource == "payments" || command.Resource == "refunds") && command.Format == "csv" && validBoardKey(command.IdempotencyKey) && (len(command.Filters) == 0 || string(command.Filters) == "{}")
}
func validBoardKey(value string) bool {
	return strings.TrimSpace(value) == value && utf8.ValidString(value) && len(value) >= 16 && len(value) <= 128
}
func validRefund(value orderport.Refund) bool {
	return value.ID > 0 && value.OrderID > 0 && value.ExternalEffectID > 0 && (value.Provider == "wechat" || value.Provider == "alipay" || value.Provider == "wechat_shop") && validText(value.OrderNo, 200) && value.OrderNo != "" && validText(value.TransactionID, 200) && value.TransactionID != "" && strings.HasPrefix(value.RefundID, "rfd_") && strings.HasPrefix(value.OutRefundNo, "rfd_") && value.RefundAmountTotal > 0 && value.Currency == "CNY" && value.Status != "" && value.ExternalEffectState != "" && !value.AutoRetryAllowed && !value.CreatedAt.IsZero()
}
func validExportJob(value orderport.ExportJob) bool {
	return strings.HasPrefix(value.JobID, "exp_") && (value.Resource == "orders" || value.Resource == "payments" || value.Resource == "refunds") && value.Format == "csv" && value.Status == "completed" && value.Operator > 0 && !value.CreatedAt.IsZero() && value.ContentText != ""
}
func validEffect(value orderport.ExternalEffect) bool {
	return value.ID > 0 && value.OrderID > 0 && (value.Provider == "wechat" || value.Provider == "alipay" || value.Provider == "wechat_shop") && (value.EffectKind == "refund" || value.EffectKind == "external_push") && (value.State == "pending_external_gate" || value.State == "outcome_unknown" || value.State == "completed" || value.State == "final_failed") && !value.AutoRetryAllowed && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}
func receiptMatches(value BoardReceipt, expected BoardReservation) bool {
	return value.ID > 0 && value.Operation == expected.Operation && value.ActorScope == expected.ActorScope && subtle.ConstantTimeCompare(value.KeyDigest[:], expected.KeyDigest[:]) == 1 && (value.State == "in_progress" || value.State == "completed")
}
func actorScope(actor int64) string { return "admin:" + fmt.Sprint(actor) }
func boardReady(value *BoardService) bool {
	return value != nil && !nilDependency(value.uow) && !nilDependency(value.store) && !nilDependency(value.events)
}
func boardClassify(err error) error {
	if err == nil {
		return ErrBoardUnavailable
	}
	switch {
	case errors.Is(err, ErrInvalidBoardCommand), errors.Is(err, ErrNotFound), errors.Is(err, ErrBoardConflict), errors.Is(err, ErrBoardUnavailable):
		return err
	default:
		return errors.Join(ErrBoardUnavailable, err)
	}
}
func jsonEquivalent(left, right []byte) bool {
	var a, b any
	return decodeBoardJSON(left, &a) == nil && decodeBoardJSON(right, &b) == nil && boardJSONValueEqual(a, b)
}

func decodeBoardJSON(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrBoardUnavailable
	}
	return nil
}

// boardJSONValueEqual compares the persisted JSONB receipt semantically.
// PostgreSQL can rewrite numeric text (for example 1.0 to 1), so byte or
// string equality would incorrectly reject an otherwise identical replay.
func boardJSONValueEqual(left, right any) bool {
	leftNumber, leftIsNumber := left.(json.Number)
	rightNumber, rightIsNumber := right.(json.Number)
	if leftIsNumber || rightIsNumber {
		if !leftIsNumber || !rightIsNumber {
			return false
		}
		leftValue, leftOK := new(big.Rat).SetString(leftNumber.String())
		rightValue, rightOK := new(big.Rat).SetString(rightNumber.String())
		return leftOK && rightOK && leftValue.Cmp(rightValue) == 0
	}
	leftObject, leftIsObject := left.(map[string]any)
	rightObject, rightIsObject := right.(map[string]any)
	if leftIsObject || rightIsObject {
		if !leftIsObject || !rightIsObject || len(leftObject) != len(rightObject) {
			return false
		}
		for key, value := range leftObject {
			other, found := rightObject[key]
			if !found || !boardJSONValueEqual(value, other) {
				return false
			}
		}
		return true
	}
	leftList, leftIsList := left.([]any)
	rightList, rightIsList := right.([]any)
	if leftIsList || rightIsList {
		if !leftIsList || !rightIsList || len(leftList) != len(rightList) {
			return false
		}
		for index := range leftList {
			if !boardJSONValueEqual(leftList[index], rightList[index]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(left, right)
}
