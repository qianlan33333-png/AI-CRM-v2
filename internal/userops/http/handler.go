package http

import (
	"errors"
	stdhttp "net/http"
	"reflect"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

const (
	minimumIdempotencyBytes = 8
	maximumIdempotencyBytes = 200
)

// Handler is an operation seam, not a router. The shared root owns route
// registration, request decoding, OpenAPI and generated transport bindings.
type Handler struct {
	application useropsport.Application
	authorizer  Authorizer
	csrf        CSRFVerifier
}

func NewHandler(application useropsport.Application, authorizer Authorizer, csrf CSRFVerifier) (*Handler, error) {
	if nilInterface(application) || nilInterface(authorizer) || nilInterface(csrf) {
		return nil, useropsport.ErrUnavailable
	}
	return &Handler{application: application, authorizer: authorizer, csrf: csrf}, nil
}

func (handler *Handler) Overview(request *stdhttp.Request, input useropsport.DirectoryQuery) (useropsport.Overview, error) {
	if _, err := handler.readActor(request); err != nil {
		return useropsport.Overview{}, err
	}
	return handler.application.Overview(request.Context(), input)
}

func (handler *Handler) ListCustomers(request *stdhttp.Request, input useropsport.DirectoryQuery) (useropsport.DirectoryPage, error) {
	if _, err := handler.readActor(request); err != nil {
		return useropsport.DirectoryPage{}, err
	}
	return handler.application.ListCustomers(request.Context(), input)
}

func (handler *Handler) GetCustomerDetail(request *stdhttp.Request, customerID domain.CustomerID) (useropsport.CustomerDetailResult, error) {
	if _, err := handler.readActor(request); err != nil {
		return useropsport.CustomerDetailResult{}, err
	}
	return handler.application.GetCustomerDetail(request.Context(), customerID)
}

func (handler *Handler) SafeExport(request *stdhttp.Request, input useropsport.SafeExportRequest) (useropsport.SafeExport, error) {
	if _, err := handler.readActor(request); err != nil {
		return useropsport.SafeExport{}, err
	}
	return handler.application.SafeExport(request.Context(), input)
}

// PreviewBatch is read-only despite accepting a body-shaped input. It never
// creates a plan, a send record, an outbound job or an external effect.
func (handler *Handler) PreviewBatch(request *stdhttp.Request, input useropsport.BatchPreviewInput) (useropsport.BatchPreview, error) {
	if _, err := handler.readActor(request); err != nil {
		return useropsport.BatchPreview{}, err
	}
	return handler.application.PreviewBatch(request.Context(), input)
}

func (handler *Handler) CreateLocalPlan(request *stdhttp.Request, input useropsport.CreateLocalPlanInput) (useropsport.LocalPlanResult, error) {
	actor, key, err := handler.writeActorAndKey(request)
	if err != nil {
		return useropsport.LocalPlanResult{}, err
	}
	input.ActorID, input.IdempotencyKey = actor.ID, key
	return handler.application.CreateLocalPlan(request.Context(), input)
}

func (handler *Handler) SetDND(request *stdhttp.Request, input useropsport.UpsertDNDInput) (useropsport.DNDMutationResult, error) {
	actor, key, err := handler.writeActorAndKey(request)
	if err != nil {
		return useropsport.DNDMutationResult{}, err
	}
	input.ActorID, input.IdempotencyKey = actor.ID, key
	return handler.application.SetDND(request.Context(), input)
}

func (handler *Handler) ClearDND(request *stdhttp.Request, input useropsport.ClearDNDInput) (useropsport.DNDMutationResult, error) {
	actor, key, err := handler.writeActorAndKey(request)
	if err != nil {
		return useropsport.DNDMutationResult{}, err
	}
	input.ActorID, input.IdempotencyKey = actor.ID, key
	return handler.application.ClearDND(request.Context(), input)
}

func (handler *Handler) ListSendRecords(request *stdhttp.Request, input useropsport.SendRecordQuery) (useropsport.SendRecordPage, error) {
	if _, err := handler.readActor(request); err != nil {
		return useropsport.SendRecordPage{}, err
	}
	return handler.application.ListSendRecords(request.Context(), input)
}

func (handler *Handler) readActor(request *stdhttp.Request) (Actor, error) {
	return handler.authorize(request, PermissionAdminRead, false)
}

func (handler *Handler) writeActorAndKey(request *stdhttp.Request) (Actor, string, error) {
	actor, err := handler.authorize(request, PermissionAdminWrite, true)
	if err != nil {
		return Actor{}, "", err
	}
	key, ok := idempotencyKey(request)
	if !ok {
		return Actor{}, "", useropsport.ErrInvalid
	}
	return actor, key, nil
}

func (handler *Handler) authorize(request *stdhttp.Request, permission Permission, csrfRequired bool) (Actor, error) {
	if handler == nil || nilInterface(handler.application) || nilInterface(handler.authorizer) || nilInterface(handler.csrf) || request == nil {
		return Actor{}, ErrAccessUnavailable
	}
	actor, err := handler.authorizer.Authorize(request.Context(), permission)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) || errors.Is(err, ErrForbidden) {
			return Actor{}, err
		}
		return Actor{}, errors.Join(ErrAccessUnavailable, err)
	}
	if actor.ID < 1 {
		return Actor{}, ErrForbidden
	}
	if csrfRequired {
		if err := handler.csrf.Verify(request); err != nil {
			if errors.Is(err, ErrCSRFInvalid) {
				return Actor{}, ErrCSRFInvalid
			}
			return Actor{}, errors.Join(ErrAccessUnavailable, err)
		}
	}
	return actor, nil
}

func idempotencyKey(request *stdhttp.Request) (string, bool) {
	if request == nil {
		return "", false
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < minimumIdempotencyBytes || len(values[0]) > maximumIdempotencyBytes || strings.TrimSpace(values[0]) != values[0] {
		return "", false
	}
	for _, character := range []byte(values[0]) {
		if character < 0x21 || character > 0x7e || character == ',' {
			return "", false
		}
	}
	return values[0], true
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
