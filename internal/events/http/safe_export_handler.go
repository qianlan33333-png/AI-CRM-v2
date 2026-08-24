package http

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const SafeExportPath = "/api/admin/internal-events/exports"

type safeExportApplication interface {
	Create(context.Context, eventapp.InternalEventSafeExportCreate) (eventapp.InternalEventSafeExport, error)
	Get(context.Context, string, int64) (eventapp.InternalEventSafeExport, error)
	Download(context.Context, string, int64) (eventapp.InternalEventSafeExport, []eventapp.InternalEventSafeExportRow, error)
}
type SafeExportHandler struct{ application safeExportApplication }

func NewSafeExportHandler(application safeExportApplication) (*SafeExportHandler, error) {
	if application == nil || (reflect.ValueOf(application).Kind() == reflect.Pointer && reflect.ValueOf(application).IsNil()) {
		return nil, errors.New("internal event safe export application is required")
	}
	return &SafeExportHandler{application}, nil
}

func (h *SafeExportHandler) Create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	actor, err := safeExportActor(r)
	if err == nil && len(r.Header.Values("Idempotency-Key")) != 1 {
		err = eventapp.ErrInternalEventSafeExportInvalid
	}
	var input struct {
		EventType string `json:"event_type"`
		Consumer  string `json:"consumer"`
		Status    string `json:"status"`
	}
	if err == nil {
		decoder := json.NewDecoder(stdhttp.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&input); err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = eventapp.ErrInternalEventSafeExportInvalid
			}
		}
		if err == nil {
			result, createErr := h.application.Create(r.Context(), eventapp.InternalEventSafeExportCreate{ActorID: actor, IdempotencyKey: r.Header.Get("Idempotency-Key"), Filter: eventapp.InternalEventSafeExportFilter{EventType: input.EventType, Consumer: input.Consumer, Status: input.Status}})
			if createErr == nil {
				writeSafeExportJSON(w, stdhttp.StatusCreated, result)
				return
			}
			err = createErr
		}
	}
	writeSafeExportError(w, r, err)
}
func (h *SafeExportHandler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	actor, err := safeExportActor(r)
	if err == nil {
		result, getErr := h.application.Get(r.Context(), safeExportID(r.URL.Path), actor)
		if getErr == nil {
			writeSafeExportJSON(w, stdhttp.StatusOK, result)
			return
		}
		err = getErr
	}
	writeSafeExportError(w, r, err)
}
func (h *SafeExportHandler) Download(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	actor, err := safeExportActor(r)
	if err == nil {
		result, rows, downloadErr := h.application.Download(r.Context(), safeExportID(strings.TrimSuffix(r.URL.Path, "/download")), actor)
		if downloadErr == nil {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=internal-event-safe-export-%s.csv", result.ID))
			w.WriteHeader(stdhttp.StatusOK)
			out := csv.NewWriter(w)
			_ = out.Write([]string{"event_id", "event_type", "occurred_at", "dispatched", "consumer", "status", "attempt_count", "completed_at"})
			for _, row := range rows {
				_ = out.Write([]string{strconv.FormatInt(int64(row.EventID), 10), safeCSV(row.EventType), row.OccurredAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), strconv.FormatBool(row.Dispatched), safeCSV(row.Consumer), safeCSV(row.Status), safeInt(row.AttemptCount), safeTime(row.CompletedAt)})
			}
			out.Flush()
			return
		}
		err = downloadErr
	}
	writeSafeExportError(w, r, err)
}
func safeExportActor(r *stdhttp.Request) (int64, error) {
	if r == nil {
		return 0, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	principal, ok := authport.PrincipalFromContext(r.Context())
	authorization, authorized := authport.AuthorizationFromContext(r.Context())
	if !ok || principal.AdminUserID < 1 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if !authorized || principal.Role != authport.RoleAdmin || authorization.Capability != authport.CapabilityAdminRead || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal.AdminUserID, nil
}
func safeExportID(path string) string { return strings.TrimPrefix(path, SafeExportPath+"/") }
func safeCSV(v string) string {
	trimmed := strings.TrimLeft(v, " \t\r\n")
	if trimmed != "" && strings.ContainsAny(trimmed[:1], "=+-@") {
		return "'" + v
	}
	return v
}
func safeInt(v *int32) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(int64(*v), 10)
}
func safeTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
}
func writeSafeExportJSON(w stdhttp.ResponseWriter, status int, result eventapp.InternalEventSafeExport) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		ID                       string `json:"id"`
		RecordCount              int    `json:"record_count"`
		Watermark                string `json:"watermark"`
		CreatedAt                string `json:"created_at"`
		DownloadURL              string `json:"download_url"`
		LocalOnly                bool   `json:"local_only"`
		RealExternalCallExecuted bool   `json:"real_external_call_executed"`
	}{result.ID, result.RecordCount, result.Watermark.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), result.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), SafeExportPath + "/" + result.ID + "/download", true, false})
}
func writeSafeExportError(w stdhttp.ResponseWriter, r *stdhttp.Request, err error) {
	var httpErr *platformhttp.HTTPError
	if errors.As(err, &httpErr) {
		platformhttp.WriteError(w, r, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, eventapp.ErrInternalEventSafeExportInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, eventapp.ErrInternalEventSafeExportNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, eventapp.ErrInternalEventSafeExportConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
}
