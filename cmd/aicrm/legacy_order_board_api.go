package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

// legacyOrderBoardApplication is deliberately the complete Order A+B boundary:
// queries, local exports, refund intent, and external-effect review. It has no
// payment-provider operation, so this compatibility surface cannot perform a
// real refund or automated retry by accident.
type legacyOrderBoardApplication interface {
	ListOrders(context.Context, orderport.BoardFilter) (orderport.Page, error)
	GetOrder(context.Context, string, string) (orderport.Detail, error)
	ListRefunds(context.Context, orderport.RefundFilter) (orderport.RefundPage, error)
	CreateExport(context.Context, orderport.ExportCommand) (orderport.ExportJob, error)
	GetExport(context.Context, string) (orderport.ExportJob, error)
	RequestRefund(context.Context, orderport.RefundCommand) (orderport.Refund, error)
	ListExternalEffects(context.Context, string, string) (orderport.ExternalEffectPage, error)
	RequestExternalEffectRetry(context.Context, int64, int64, string) (orderport.ExternalEffect, error)
}

func (handler *Handler) ListOrderBoard(writer http.ResponseWriter, request *http.Request) {
	// Keep the previously closed I03 list contract independently testable while
	// the complete board is composed only in the production API component.
	if handler != nil && handler.orderBoard == nil {
		handler.ListOrders(writer, request)
		return
	}
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	filter, err := legacyBoardOrderFilter(request, "all", orderapp.DefaultLimit)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	page, err := board.ListOrders(request.Context(), filter)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *Handler) ListAlipayTransactions(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	filter, err := legacyBoardOrderFilter(request, "alipay", orderapp.DefaultLimit)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	page, err := board.ListOrders(request.Context(), filter)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *Handler) GetAlipayTransaction(writer http.ResponseWriter, request *http.Request) {
	handler.getOrderBoard(writer, request, "alipay", "order_no")
}

func (handler *Handler) GetOrderBoard(writer http.ResponseWriter, request *http.Request) {
	handler.getOrderBoard(writer, request, "", "order_no")
}

func (handler *Handler) getOrderBoard(writer http.ResponseWriter, request *http.Request, fixedProvider, pathName string) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	query := request.URL.Query()
	if !singleOrderBoardQueryValues(query, map[string]bool{"provider": true}) || (query.Get("provider") != "" && fixedProvider != "") {
		writeOrderError(writer, orderapp.ErrInvalidBoardCommand)
		return
	}
	provider := fixedProvider
	if provider == "" {
		provider = query.Get("provider")
		if provider == "" {
			provider = "auto"
		}
	}
	detail, err := board.GetOrder(request.Context(), provider, chi.URLParam(request, pathName))
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, detail)
}

func (handler *Handler) GetOrderBoardItems(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	query := request.URL.Query()
	if !singleOrderBoardQueryValues(query, map[string]bool{"provider": true}) {
		writeOrderError(writer, orderapp.ErrInvalidBoardCommand)
		return
	}
	provider := query.Get("provider")
	if provider == "" {
		provider = "auto"
	}
	detail, err := board.GetOrder(request.Context(), provider, chi.URLParam(request, "order_no"))
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	// The frozen evidence proves an item-list route but not a separate item
	// schema. A v2 order projection is one immutable purchased-product snapshot,
	// so expose that actual item rather than inventing a second DTO.
	writeJSON(writer, http.StatusOK, map[string]any{"items": []orderport.Item{detail.Item}})
}

func (handler *Handler) ListWechatTransactions(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	filter, err := legacyBoardOrderFilter(request, "wechat", 20)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	page, err := board.ListOrders(request.Context(), filter)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *Handler) ListOrderBoardRefunds(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	filter, err := legacyBoardRefundFilter(request)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	page, err := board.ListRefunds(request.Context(), filter)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *Handler) CreateOrderBoardExport(writer http.ResponseWriter, request *http.Request) {
	handler.createOrderBoardExport(writer, request, false)
}

func (handler *Handler) CreateWechatOrderBoardExport(writer http.ResponseWriter, request *http.Request) {
	handler.createOrderBoardExport(writer, request, true)
}

func (handler *Handler) createOrderBoardExport(writer http.ResponseWriter, request *http.Request, wechatOnly bool) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	principal, key, err := orderBoardActorAndKey(request)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	command, err := legacyExportCommand(writer, request, principal.AdminUserID, key, wechatOnly)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	result, err := board.CreateExport(request.Context(), command)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) GetOrderBoardExport(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	if len(request.URL.Query()) != 0 {
		writeOrderError(writer, orderapp.ErrInvalidBoardCommand)
		return
	}
	result, err := board.GetExport(request.Context(), chi.URLParam(request, "job_id"))
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) DeprecatedWechatOrderBoardExport(writer http.ResponseWriter, _ *http.Request) {
	// These two routes are explicitly marked deprecated in the frozen mapping.
	// Keep them registered and fail closed; they must not redirect to, or invoke,
	// a legacy export worker.
	writeJSON(writer, http.StatusGone, map[string]string{"error_code": "deprecated", "message": "wechat order export route is deprecated"})
}

func (handler *Handler) CreateOrderBoardRefund(writer http.ResponseWriter, request *http.Request) {
	handler.createOrderBoardRefund(writer, request, "", "")
}

func (handler *Handler) CreateWechatOrderBoardRefund(writer http.ResponseWriter, request *http.Request) {
	handler.createOrderBoardRefund(writer, request, "wechat", chi.URLParam(request, "order_id"))
}

func (handler *Handler) createOrderBoardRefund(writer http.ResponseWriter, request *http.Request, fixedProvider, fixedOrderReference string) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	principal, key, err := orderBoardActorAndKey(request)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	command, err := legacyRefundCommand(writer, request, principal.AdminUserID, key, fixedProvider, fixedOrderReference)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	result, err := board.RequestRefund(request.Context(), command)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ListWechatOrderExternalEffects(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	if len(request.URL.Query()) != 0 {
		writeOrderError(writer, orderapp.ErrInvalidBoardCommand)
		return
	}
	page, err := board.ListExternalEffects(request.Context(), "wechat", chi.URLParam(request, "order_id"))
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *Handler) ReviewWechatOrderExternalEffect(writer http.ResponseWriter, request *http.Request) {
	board := handler.orderBoardOrFail(writer)
	if board == nil {
		return
	}
	principal, key, err := orderBoardActorAndKey(request)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	if len(request.URL.Query()) != 0 || !emptyOrderBoardBody(writer, request) {
		writeOrderError(writer, orderapp.ErrInvalidBoardCommand)
		return
	}
	deliveryID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "delivery_id")), 10, 64)
	if err != nil || deliveryID < 1 {
		writeOrderError(writer, orderapp.ErrInvalidBoardCommand)
		return
	}
	page, err := board.ListExternalEffects(request.Context(), "wechat", chi.URLParam(request, "order_id"))
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	found := false
	for _, effect := range page.Items {
		if effect.ID == deliveryID {
			found = true
			break
		}
	}
	if !found {
		writeOrderError(writer, orderapp.ErrNotFound)
		return
	}
	result, err := board.RequestExternalEffectRetry(request.Context(), deliveryID, principal.AdminUserID, key)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) orderBoardOrFail(writer http.ResponseWriter) legacyOrderBoardApplication {
	if handler == nil || handler.orderBoard == nil {
		writeOrderError(writer, orderapp.ErrBoardUnavailable)
		return nil
	}
	return handler.orderBoard
}

func legacyBoardOrderFilter(request *http.Request, provider string, defaultLimit int32) (orderport.BoardFilter, error) {
	if request == nil {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	query := request.URL.Query()
	allowed := map[string]bool{"provider": true, "status": true, "payment_status": true, "product_code": true, "mobile": true, "external_userid": true, "identity": true, "transaction_id": true, "platform_transaction_no": true, "order_no": true, "out_trade_no": true, "created_from": true, "created_to": true, "date_from": true, "date_to": true, "limit": true, "offset": true, "cursor": true}
	if provider == "alipay" {
		allowed = map[string]bool{"payment_status": true, "product_code": true, "mobile": true, "external_userid": true, "date_from": true, "date_to": true, "limit": true, "offset": true}
	}
	if provider == "wechat" {
		allowed = map[string]bool{"mobile": true, "identity": true, "transaction_id": true, "product_code": true, "created_from": true, "created_to": true, "status": true, "limit": true, "offset": true}
	}
	if !singleOrderBoardQueryValues(query, allowed) {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	if provider == "all" {
		provider = query.Get("provider")
		if provider == "" {
			provider = "all"
		}
	} else if query.Get("provider") != "" {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	status, ok := sameOrderBoardAlias(query.Get("status"), query.Get("payment_status"))
	if !ok {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	identity, ok := sameOrderBoardAlias(query.Get("identity"), query.Get("external_userid"))
	if !ok {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	transactionID, ok := sameOrderBoardAlias(query.Get("transaction_id"), query.Get("platform_transaction_no"))
	if !ok {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	orderNo, ok := sameOrderBoardAlias(query.Get("order_no"), query.Get("out_trade_no"))
	if !ok {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	createdFrom, ok := sameOrderBoardAlias(query.Get("created_from"), query.Get("date_from"))
	if !ok {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	createdTo, ok := sameOrderBoardAlias(query.Get("created_to"), query.Get("date_to"))
	if !ok {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	limit, err := parseOrderPageValue(query.Get("limit"), defaultLimit)
	var offset int32
	if err == nil {
		offset, err = parseOrderPageValue(query.Get("offset"), 0)
		if query.Get("cursor") != "" && offset != 0 {
			err = errors.New("cursor cannot be combined with nonzero offset")
		}
	}
	if err != nil || query.Get("cursor") != "" {
		// The evidence proves the optional opaque parameter but not its encoding.
		// Do not fabricate a cursor codec or silently apply the wrong page.
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	from, err := parseLegacyOrderTime(createdFrom)
	if err != nil {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	to, err := parseLegacyOrderTime(createdTo)
	if err != nil {
		return orderport.BoardFilter{}, orderapp.ErrInvalidBoardCommand
	}
	return orderport.BoardFilter{Provider: provider, Status: status, ProductCode: query.Get("product_code"), Mobile: query.Get("mobile"), Identity: identity, TransactionID: transactionID, OrderNo: orderNo, CreatedFrom: from, CreatedTo: to, Limit: limit, Offset: offset}, nil
}

func legacyBoardRefundFilter(request *http.Request) (orderport.RefundFilter, error) {
	if request == nil {
		return orderport.RefundFilter{}, orderapp.ErrInvalidBoardCommand
	}
	query := request.URL.Query()
	allowed := map[string]bool{"provider": true, "order_no": true, "out_trade_no": true, "transaction_id": true, "refund_id": true, "out_refund_no": true, "status": true, "created_from": true, "created_to": true, "limit": true, "offset": true}
	if !singleOrderBoardQueryValues(query, allowed) {
		return orderport.RefundFilter{}, orderapp.ErrInvalidBoardCommand
	}
	orderNo, ok := sameOrderBoardAlias(query.Get("order_no"), query.Get("out_trade_no"))
	if !ok {
		return orderport.RefundFilter{}, orderapp.ErrInvalidBoardCommand
	}
	limit, err := parseOrderPageValue(query.Get("limit"), orderapp.DefaultLimit)
	if err == nil {
		var offset int32
		offset, err = parseOrderPageValue(query.Get("offset"), 0)
		if err == nil {
			from, fromErr := parseLegacyOrderTime(query.Get("created_from"))
			to, toErr := parseLegacyOrderTime(query.Get("created_to"))
			if fromErr != nil || toErr != nil {
				err = orderapp.ErrInvalidBoardCommand
			} else {
				return orderport.RefundFilter{Provider: query.Get("provider"), OrderNo: orderNo, TransactionID: query.Get("transaction_id"), RefundID: query.Get("refund_id"), OutRefundNo: query.Get("out_refund_no"), Status: query.Get("status"), CreatedFrom: from, CreatedTo: to, Limit: limit, Offset: offset}, nil
			}
		}
	}
	return orderport.RefundFilter{}, orderapp.ErrInvalidBoardCommand
}

func sameOrderBoardAlias(left, right string) (string, bool) {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left != "" && right != "" && left != right {
		return "", false
	}
	if left != "" {
		return left, true
	}
	return right, true
}

func singleOrderBoardQueryValues(query map[string][]string, allowed map[string]bool) bool {
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return false
		}
	}
	return true
}

type legacyOrderExportRequest struct {
	Resource string `json:"resource"`
	Format   string `json:"format"`
}

func legacyExportCommand(writer http.ResponseWriter, request *http.Request, actor int64, key string, wechatOnly bool) (orderport.ExportCommand, error) {
	if wechatOnly {
		if !emptyOrderBoardBody(writer, request) {
			return orderport.ExportCommand{}, orderapp.ErrInvalidBoardCommand
		}
		return orderport.ExportCommand{Resource: "orders", Format: "csv", Actor: actor, IdempotencyKey: key}, nil
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var body legacyOrderExportRequest
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return orderport.ExportCommand{}, orderapp.ErrInvalidBoardCommand
	}
	return orderport.ExportCommand{Resource: strings.TrimSpace(body.Resource), Format: strings.TrimSpace(body.Format), Actor: actor, IdempotencyKey: key}, nil
}

type legacyOrderRefundRequest struct {
	Provider                  string `json:"provider"`
	OrderNo                   string `json:"order_no"`
	RefundAmountTotal         *int64 `json:"refund_amount_total"`
	Reason                    string `json:"reason"`
	TransactionIDConfirmation string `json:"transaction_id_confirmation"`
	Checked                   *bool  `json:"checked"`
	Operator                  string `json:"operator"`
}

func legacyRefundCommand(writer http.ResponseWriter, request *http.Request, actor int64, key, fixedProvider, fixedOrderReference string) (orderport.RefundCommand, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var body legacyOrderRefundRequest
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) || body.RefundAmountTotal == nil || body.Checked == nil {
		return orderport.RefundCommand{}, orderapp.ErrInvalidBoardCommand
	}
	provider, reference := strings.TrimSpace(body.Provider), strings.TrimSpace(body.OrderNo)
	if fixedProvider != "" {
		if provider != "" && provider != fixedProvider {
			return orderport.RefundCommand{}, orderapp.ErrInvalidBoardCommand
		}
		provider = fixedProvider
	}
	if fixedOrderReference != "" {
		if reference != "" && reference != fixedOrderReference {
			return orderport.RefundCommand{}, orderapp.ErrInvalidBoardCommand
		}
		reference = fixedOrderReference
	}
	// The legacy `operator` field is accepted for transport compatibility but is
	// never trusted: the authenticated admin identity is the sole business actor.
	return orderport.RefundCommand{Provider: provider, OrderReference: reference, RefundAmountTotal: *body.RefundAmountTotal, Reason: strings.TrimSpace(body.Reason), TransactionIDConfirmation: strings.TrimSpace(body.TransactionIDConfirmation), Checked: *body.Checked, Actor: actor, IdempotencyKey: key}, nil
}

func orderBoardActorAndKey(request *http.Request) (authport.Principal, string, error) {
	if request == nil {
		return authport.Principal{}, "", orderapp.ErrInvalidBoardCommand
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, "", authport.ErrUnauthorized
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || strings.TrimSpace(values[0]) != values[0] || len(values[0]) < 16 || len(values[0]) > 128 {
		return authport.Principal{}, "", orderapp.ErrInvalidBoardCommand
	}
	return principal, values[0], nil
}

func emptyOrderBoardBody(writer http.ResponseWriter, request *http.Request) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	var value map[string]json.RawMessage
	return decoder.Decode(&value) == nil && len(value) == 0 && errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}
