package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	CheckoutPath        = "/api/v1/wechat-pay/checkouts"
	PaymentCallbackPath = "/api/public/wechat-pay/callbacks/payment"
	RefundCallbackPath  = "/api/public/wechat-pay/callbacks/refund"
	maxBody             = 128 << 10
)

type Handler struct {
	application orderport.SettlementApplication
	callbacks   orderport.CallbackVerifier
	actorKey    []byte
}

func NewHandler(application orderport.SettlementApplication, callbacks orderport.CallbackVerifier, actorKey []byte) (*Handler, error) {
	if application == nil || callbacks == nil || len(actorKey) < 32 {
		return nil, orderport.ErrSettlementUnavailable
	}
	return &Handler{application: application, callbacks: callbacks, actorKey: append([]byte(nil), actorKey...)}, nil
}

func (handler *Handler) Checkout(writer http.ResponseWriter, request *http.Request) {
	actor, identity, err := handler.actor(request, authport.CapabilityOrderWrite)
	var input struct {
		CustomerID  int64                 `json:"customer_id"`
		ProductID   int64                 `json:"product_id"`
		ProductKind orderport.ProductKind `json:"product_kind"`
	}
	if err == nil {
		err = decode(writer, request, &input)
	}
	if err == nil && len(request.Header.Values("Idempotency-Key")) == 1 {
		result, callErr := handler.application.Checkout(request.Context(), orderport.CheckoutCommand{CustomerID: input.CustomerID, ProductID: input.ProductID, ProductKind: input.ProductKind, PaymentIdentityDigest: identity, ActorScope: actor, IdempotencyKey: request.Header.Get("Idempotency-Key")})
		if callErr == nil {
			writeJSON(writer, http.StatusCreated, result)
			return
		}
		err = callErr
	} else if err == nil {
		err = orderport.ErrInvalidSettlement
	}
	writeError(writer, request, err)
}

func (handler *Handler) Get(writer http.ResponseWriter, request *http.Request, merchantOrderNo string) {
	_, identity, err := handler.actor(request, authport.CapabilityOrderRead)
	if err == nil {
		result, callErr := handler.application.GetSelfScoped(request.Context(), merchantOrderNo, identity)
		if callErr == nil {
			writeJSON(writer, http.StatusOK, result)
			return
		}
		err = callErr
	}
	writeError(writer, request, err)
}

func (handler *Handler) Refund(writer http.ResponseWriter, request *http.Request, orderID int64) {
	principal, err := adminActor(request, authport.CapabilityOrderWrite)
	var input struct {
		AmountMinor               int64  `json:"amount_minor"`
		Reason                    string `json:"reason"`
		TransactionIDConfirmation string `json:"transaction_id_confirmation"`
	}
	if err == nil {
		err = decode(writer, request, &input)
	}
	if err == nil && len(request.Header.Values("Idempotency-Key")) == 1 {
		result, callErr := handler.application.RequestRefundV2(request.Context(), orderport.RefundCommandV2{OrderID: orderport.ID(orderID), AmountMinor: input.AmountMinor, Reason: input.Reason, TransactionIDConfirmation: input.TransactionIDConfirmation, Actor: principal.AdminUserID, IdempotencyKey: request.Header.Get("Idempotency-Key")})
		if callErr == nil {
			writeJSON(writer, http.StatusAccepted, result)
			return
		}
		err = callErr
	} else if err == nil {
		err = orderport.ErrInvalidSettlement
	}
	writeError(writer, request, err)
}

func (handler *Handler) PaymentCallback(writer http.ResponseWriter, request *http.Request) {
	body, headers, err := callbackInput(writer, request)
	if err == nil {
		command, verifyErr := handler.callbacks.VerifyPayment(request.Context(), body, headers)
		if verifyErr == nil {
			_, applyErr := handler.application.ApplyPaymentCallback(request.Context(), command)
			if applyErr == nil {
				writeJSON(writer, http.StatusOK, map[string]string{"code": "SUCCESS"})
				return
			}
			err = applyErr
		} else {
			err = errors.Join(orderport.ErrInvalidSettlement, verifyErr)
		}
	}
	writeError(writer, request, err)
}

func (handler *Handler) RefundCallback(writer http.ResponseWriter, request *http.Request) {
	body, headers, err := callbackInput(writer, request)
	if err == nil {
		command, verifyErr := handler.callbacks.VerifyRefund(request.Context(), body, headers)
		if verifyErr == nil {
			_, applyErr := handler.application.ApplyRefundCallback(request.Context(), command)
			if applyErr == nil {
				writeJSON(writer, http.StatusOK, map[string]string{"code": "SUCCESS"})
				return
			}
			err = applyErr
		} else {
			err = errors.Join(orderport.ErrInvalidSettlement, verifyErr)
		}
	}
	writeError(writer, request, err)
}

func (handler *Handler) actor(request *http.Request, capability authport.Capability) (string, [32]byte, error) {
	principal, err := adminActor(request, capability)
	if err != nil {
		return "", [32]byte{}, err
	}
	session, ok := authport.SessionFromContext(request.Context())
	if !ok {
		return "", [32]byte{}, authport.ErrUnauthenticated
	}
	digest := handler.hmac("pe01/payment-identity/v1", strconv.FormatInt(principal.AdminUserID, 10), string(session))
	actorDigest := handler.hmac("pe01/actor-scope/v1", strconv.FormatInt(principal.AdminUserID, 10), string(session))
	return "payment-session:" + hex.EncodeToString(actorDigest[:]), digest, nil
}

func (handler *Handler) hmac(domain string, values ...string) [32]byte {
	mac := hmac.New(sha256.New, handler.actorKey)
	_, _ = mac.Write([]byte(domain + "\x00" + strings.Join(values, "\x00")))
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func adminActor(request *http.Request, capability authport.Capability) (authport.Principal, error) {
	if request == nil {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	authorization, authorized := authport.AuthorizationFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	if !authorized || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, authport.ErrUnauthorized
	}
	return principal, nil
}

func decode(writer http.ResponseWriter, request *http.Request, destination any) error {
	if request == nil || request.Body == nil || len(request.Header.Values("Content-Type")) != 1 || request.Header.Get("Content-Type") != "application/json" {
		return orderport.ErrInvalidSettlement
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return orderport.ErrInvalidSettlement
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return orderport.ErrInvalidSettlement
	}
	return nil
}

func callbackInput(writer http.ResponseWriter, request *http.Request) ([]byte, map[string]string, error) {
	if request == nil || request.Body == nil || len(request.Header.Values("Content-Type")) != 1 || request.Header.Get("Content-Type") != "application/json" {
		return nil, nil, orderport.ErrInvalidSettlement
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxBody))
	if err != nil || len(body) == 0 || !json.Valid(body) {
		return nil, nil, orderport.ErrInvalidSettlement
	}
	headers := make(map[string]string, 4)
	for _, name := range []string{"Wechatpay-Timestamp", "Wechatpay-Nonce", "Wechatpay-Serial", "Wechatpay-Signature"} {
		if len(request.Header.Values(name)) != 1 || request.Header.Get(name) == "" {
			return nil, nil, orderport.ErrInvalidSettlement
		}
		headers[name] = request.Header.Get(name)
	}
	return body, headers, nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, authport.ErrUnauthenticated):
		code = platformhttp.CodeUnauthenticated
	case errors.Is(err, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	case errors.Is(err, orderport.ErrInvalidSettlement):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, orderport.ErrSettlementNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, orderport.ErrSettlementConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}
