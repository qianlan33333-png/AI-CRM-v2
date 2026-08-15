package main

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func (handler *Handler) ListOrders(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.orders == nil || request == nil {
		writeOrderError(writer, orderapp.ErrUnavailable)
		return
	}
	filter, err := legacyOrderFilter(request)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	page, err := handler.orders.List(request.Context(), filter)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func legacyOrderFilter(request *http.Request) (orderport.Filter, error) {
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return orderport.Filter{}, orderapp.ErrInvalidArgument
	}
	allowed := map[string]bool{
		"provider": true, "order_no": true, "mobile": true, "product_code": true,
		"created_from": true, "created_to": true, "status": true, "limit": true, "offset": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return orderport.Filter{}, orderapp.ErrInvalidArgument
		}
	}
	filter := orderport.Filter{
		Provider: query.Get("provider"), OrderNo: query.Get("order_no"), Mobile: query.Get("mobile"),
		ProductCode: query.Get("product_code"), Status: query.Get("status"),
	}
	filter.Limit, err = parseOrderPageValue(query.Get("limit"), orderapp.DefaultLimit)
	if err == nil {
		filter.Offset, err = parseOrderPageValue(query.Get("offset"), 0)
	}
	if err == nil {
		filter.CreatedFrom, err = parseLegacyOrderTime(query.Get("created_from"))
	}
	if err == nil {
		filter.CreatedTo, err = parseLegacyOrderTime(query.Get("created_to"))
	}
	if err != nil {
		return orderport.Filter{}, orderapp.ErrInvalidArgument
	}
	provider := strings.ToLower(strings.TrimSpace(filter.Provider))
	if provider != "" && provider != "all" && provider != "wechat" && provider != "alipay" && provider != "wechat_shop" ||
		filter.Limit < 1 || filter.Limit > orderapp.MaximumLimit || filter.Offset < 0 || filter.Offset > orderapp.MaximumOffset ||
		filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedFrom.After(*filter.CreatedTo) ||
		!validOrderQueryText(filter.OrderNo, 200) || !validOrderQueryText(filter.Mobile, 80) ||
		!validOrderQueryText(filter.ProductCode, 200) || !validOrderQueryText(filter.Status, 80) {
		return orderport.Filter{}, orderapp.ErrInvalidArgument
	}
	return filter, nil
}

func validOrderQueryText(value string, maximum int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(strings.TrimSpace(value)) <= maximum
}

func parseOrderPageValue(raw string, fallback int32) (int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	return int32(value), err
}

func parseLegacyOrderTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04", "2006-01-02"} {
		if value, err := time.Parse(layout, raw); err == nil {
			value = value.UTC()
			return &value, nil
		}
	}
	return nil, orderapp.ErrInvalidArgument
}

func writeOrderError(writer http.ResponseWriter, err error) {
	status, code, compatibility := http.StatusServiceUnavailable, "unavailable", platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, orderapp.ErrInvalidArgument):
		status, code, compatibility = http.StatusBadRequest, "invalid_argument", platformhttp.CodeMalformedRequest
	case errors.Is(err, authport.ErrUnauthenticated):
		status, code, compatibility = http.StatusUnauthorized, "unauthorized", platformhttp.CodeUnauthenticated
	case errors.Is(err, authport.ErrUnauthorized):
		status, code, compatibility = http.StatusForbidden, "forbidden", platformhttp.CodeUnauthorized
	case errors.Is(err, orderapp.ErrNotFound):
		status, code, compatibility = http.StatusNotFound, "not_found", platformhttp.CodeNotFound
	case errors.Is(err, orderapp.ErrConflict):
		status, code, compatibility = http.StatusConflict, "conflict", platformhttp.CodeConflict
	}
	platformhttp.MarkCompatibilityError(writer, compatibility)
	message := "order list unavailable"
	if status != http.StatusServiceUnavailable {
		message = err.Error()
	}
	writeJSON(writer, status, map[string]string{"message": message, "error_code": code})
}
