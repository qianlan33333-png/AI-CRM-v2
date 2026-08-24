package http

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerSafeExportApplication interface {
	Create(context.Context, contactapp.CustomerSafeExportCreate) (contactapp.CustomerSafeExport, error)
	Get(context.Context, string, int64) (contactapp.CustomerSafeExport, error)
	Download(context.Context, string, int64, *int64) (contactapp.CustomerSafeExport, []contactapp.CustomerSafeExportRow, error)
}

type CustomerSafeExportHandler struct{ application customerSafeExportApplication }

func NewCustomerSafeExportHandler(application customerSafeExportApplication) (*CustomerSafeExportHandler, error) {
	if application == nil || (reflect.ValueOf(application).Kind() == reflect.Pointer && reflect.ValueOf(application).IsNil()) {
		return nil, errors.New("customer safe export application is required")
	}
	return &CustomerSafeExportHandler{application: application}, nil
}

func (handler *CustomerSafeExportHandler) Create(writer http.ResponseWriter, request *http.Request, params generated.CreateCustomerSafeExportParams) {
	actorID, scope, err := customerSafeExportActor(request)
	if err == nil && (handler == nil || handler.application == nil || len(request.Header.Values("Idempotency-Key")) != 1) {
		err = contactapp.ErrCustomerSafeExportInvalid
	}
	if err == nil {
		var filter contactapp.CustomerListInput
		filter, err = decodeCustomerSafeExportFilter(writer, request)
		if err == nil && scope != nil && filter.OwnerStaffID != nil && *scope != *filter.OwnerStaffID {
			err = platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		if err == nil {
			export, createErr := handler.application.Create(request.Context(), contactapp.CustomerSafeExportCreate{ActorID: actorID, OwnerScopeStaffID: scope, IdempotencyKey: string(params.IdempotencyKey), Filter: filter})
			if createErr == nil {
				writeCustomerSafeExportJSON(writer, http.StatusCreated, export)
				return
			}
			err = createErr
		}
	}
	writeCustomerSafeExportError(writer, request, err)
}

func (handler *CustomerSafeExportHandler) Get(writer http.ResponseWriter, request *http.Request, exportID generated.CustomerSafeExportID) {
	actorID, _, err := customerSafeExportActor(request)
	if err == nil && (handler == nil || handler.application == nil) {
		err = contactapp.ErrCustomerSafeExportUnavailable
	}
	if err == nil {
		export, getErr := handler.application.Get(request.Context(), string(exportID), actorID)
		if getErr == nil {
			writeCustomerSafeExportJSON(writer, http.StatusOK, export)
			return
		}
		err = getErr
	}
	writeCustomerSafeExportError(writer, request, err)
}

func (handler *CustomerSafeExportHandler) Download(writer http.ResponseWriter, request *http.Request, exportID generated.CustomerSafeExportID) {
	actorID, scope, err := customerSafeExportActor(request)
	if err == nil && (handler == nil || handler.application == nil) {
		err = contactapp.ErrCustomerSafeExportUnavailable
	}
	if err == nil {
		_, rows, downloadErr := handler.application.Download(request.Context(), string(exportID), actorID, scope)
		if downloadErr == nil {
			writer.Header().Set("Cache-Control", "no-store")
			writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
			writer.Header().Set("X-Content-Type-Options", "nosniff")
			writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=customer-safe-export-%s.csv", exportID))
			writer.WriteHeader(http.StatusOK)
			csvWriter := csv.NewWriter(writer)
			_ = csvWriter.Write([]string{"customer_id", "display_name", "owner_staff_id", "stage_id", "channel_id", "added_at", "last_interact_at"})
			for _, row := range rows {
				_ = csvWriter.Write([]string{strconv.FormatInt(row.CustomerID, 10), csvSafe(row.DisplayName), csvInt(row.OwnerStaffID), csvInt(row.StageID), csvInt(row.ChannelID), csvTime(row.AddedAt), csvTime(row.LastInteractAt)})
			}
			csvWriter.Flush()
			return
		}
		err = downloadErr
	}
	writeCustomerSafeExportError(writer, request, err)
}

func customerSafeExportActor(request *http.Request) (int64, *int64, error) {
	if request == nil {
		return 0, nil, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 {
		return 0, nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if !authorizationOK || authorization.Capability != authport.CapabilityCustomersRead {
		return 0, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return 0, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return principal.AdminUserID, nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil || *principal.StaffID != authorization.OwnerStaffID {
			return 0, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		value := authorization.OwnerStaffID
		return principal.AdminUserID, &value, nil
	default:
		return 0, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func decodeCustomerSafeExportFilter(writer http.ResponseWriter, request *http.Request) (contactapp.CustomerListInput, error) {
	object, err := decodeCustomerMutationObject(writer, request)
	if err != nil || len(object) > 9 {
		return contactapp.CustomerListInput{}, contactapp.ErrCustomerSafeExportInvalid
	}
	allowed := map[string]bool{"owner_staff_id": true, "stage_id": true, "channel_id": true, "tag_id": true, "keyword": true, "added_after": true, "added_before": true, "last_interact_after": true, "last_interact_before": true}
	for key := range object {
		if !allowed[key] {
			return contactapp.CustomerListInput{}, contactapp.ErrCustomerSafeExportInvalid
		}
	}
	result := contactapp.CustomerListInput{}
	if result.OwnerStaffID, err = customerSafeExportInteger(object, "owner_staff_id"); err != nil {
		return result, err
	}
	if result.StageID, err = customerSafeExportInteger(object, "stage_id"); err != nil {
		return result, err
	}
	if result.ChannelID, err = customerSafeExportInteger(object, "channel_id"); err != nil {
		return result, err
	}
	if result.TagID, err = customerSafeExportInteger(object, "tag_id"); err != nil {
		return result, err
	}
	if raw, ok := object["keyword"]; ok {
		if err = json.Unmarshal(raw, &result.Keyword); err != nil {
			return result, contactapp.ErrCustomerSafeExportInvalid
		}
	}
	if result.AddedAfter, err = customerSafeExportTime(object, "added_after"); err != nil {
		return result, err
	}
	if result.AddedBefore, err = customerSafeExportTime(object, "added_before"); err != nil {
		return result, err
	}
	if result.LastInteractAfter, err = customerSafeExportTime(object, "last_interact_after"); err != nil {
		return result, err
	}
	if result.LastInteractBefore, err = customerSafeExportTime(object, "last_interact_before"); err != nil {
		return result, err
	}
	return result, nil
}

func customerSafeExportInteger(object map[string]json.RawMessage, key string) (*int64, error) {
	raw, ok := object[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	value, err := decodeNullableInteger(raw, 64)
	if err != nil || value == nil || *value < 1 {
		return nil, contactapp.ErrCustomerSafeExportInvalid
	}
	return value, nil
}

func customerSafeExportTime(object map[string]json.RawMessage, key string) (*time.Time, error) {
	raw, ok := object[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var value time.Time
	if err := json.Unmarshal(raw, &value); err != nil || value.IsZero() {
		return nil, contactapp.ErrCustomerSafeExportInvalid
	}
	value = value.UTC()
	return &value, nil
}

func writeCustomerSafeExportJSON(writer http.ResponseWriter, status int, export contactapp.CustomerSafeExport) {
	if export.ID == "" || export.RecordCount < 0 || export.RecordCount > contactapp.CustomerSafeExportMaximumRows || export.Watermark.IsZero() || export.CreatedAt.IsZero() {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(generated.CustomerSafeExportResponse{Id: export.ID, RecordCount: export.RecordCount, Watermark: export.Watermark.UTC(), CreatedAt: export.CreatedAt.UTC(), DownloadUrl: "/api/v1/customer-exports/" + export.ID + "/download", LocalOnly: generated.CustomerSafeExportResponseLocalOnlyTrue, RealExternalCallExecuted: generated.CustomerSafeExportResponseRealExternalCallExecutedFalse})
}

func writeCustomerSafeExportError(writer http.ResponseWriter, request *http.Request, err error) {
	var httpError *platformhttp.HTTPError
	if errors.As(err, &httpError) {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrCustomerSafeExportInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, contactapp.ErrCustomerSafeExportNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrCustomerSafeExportConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func csvSafe(value string) string {
	if value != "" && strings.ContainsAny(value[:1], "=+-@") {
		return "'" + value
	}
	return value
}
func csvInt(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}
func csvTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
