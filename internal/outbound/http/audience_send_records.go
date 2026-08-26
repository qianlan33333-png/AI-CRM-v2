package outboundhttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const AudienceSendRecordsPathPrefix = "/api/admin/ai-audience/packages/"

type AudienceSendRecordApplication interface {
	List(context.Context, int64, int32, int32) (outboundapp.AudienceSendRecordPage, error)
	Get(context.Context, int64, int64) (outboundport.AudienceSendRecord, error)
}

type AudienceSendRecordHandler struct {
	application AudienceSendRecordApplication
}

func NewAudienceSendRecordHandler(application AudienceSendRecordApplication) (*AudienceSendRecordHandler, error) {
	if application == nil {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	return &AudienceSendRecordHandler{application: application}, nil
}

func (handler *AudienceSendRecordHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil || request.Method != http.MethodGet || request.URL.RawPath != "" {
		writeAudienceSendRecordError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	if _, err := campaignHandoffActor(request.Context(), authport.CapabilitySegmentsRead); err != nil {
		writeAudienceSendRecordError(writer, request, err)
		return
	}
	packageID, recordID, detail, ok := audienceSendRecordPath(request.URL.Path)
	if !ok {
		writeAudienceSendRecordError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	if detail {
		if request.URL.RawQuery != "" {
			writeAudienceSendRecordError(writer, request, outbound.ErrCampaignDispatchInvalid)
			return
		}
		record, err := handler.application.Get(request.Context(), packageID, recordID)
		if err != nil {
			writeAudienceSendRecordError(writer, request, err)
			return
		}
		writeCampaignHandoffJSON(writer, http.StatusOK, map[string]any{"ok": true, "record": audienceSendRecordResponseOf(record)})
		return
	}
	limit, offset, ok := audienceSendRecordPage(request.URL.Query())
	if !ok {
		writeAudienceSendRecordError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	page, err := handler.application.List(request.Context(), packageID, limit, offset)
	if err != nil {
		writeAudienceSendRecordError(writer, request, err)
		return
	}
	items := make([]audienceSendRecordResponse, len(page.Items))
	for index, record := range page.Items {
		items[index] = audienceSendRecordResponseOf(record)
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, map[string]any{"ok": true, "items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}

func audienceSendRecordPath(path string) (packageID, recordID int64, detail, ok bool) {
	if !strings.HasPrefix(path, AudienceSendRecordsPathPrefix) || strings.HasSuffix(path, "/") || strings.Contains(path, "//") || strings.Contains(path, "\\") {
		return 0, 0, false, false
	}
	parts := strings.Split(strings.TrimPrefix(path, AudienceSendRecordsPathPrefix), "/")
	if len(parts) != 2 && len(parts) != 3 || parts[1] != "send-records" {
		return 0, 0, false, false
	}
	packageID, ok = positiveAudienceSendRecordID(parts[0])
	if !ok || len(parts) == 2 {
		return packageID, 0, false, ok
	}
	recordText := parts[2]
	if strings.HasPrefix(recordText, "campaign:") {
		recordText = strings.TrimPrefix(recordText, "campaign:")
	}
	recordID, ok = positiveAudienceSendRecordID(recordText)
	return packageID, recordID, true, ok
}

func positiveAudienceSendRecordID(value string) (int64, bool) {
	if value == "" || value[0] == '0' || strings.TrimSpace(value) != value {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}

func audienceSendRecordPage(query url.Values) (int32, int32, bool) {
	for key, values := range query {
		if (key != "limit" && key != "offset") || len(values) != 1 {
			return 0, 0, false
		}
	}
	limit, offset := int64(outboundapp.DefaultAudienceSendRecordLimit), int64(0)
	var err error
	if raw := query.Get("limit"); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 32)
	}
	if err == nil {
		if raw := query.Get("offset"); raw != "" {
			offset, err = strconv.ParseInt(raw, 10, 32)
		}
	}
	return int32(limit), int32(offset), err == nil && limit >= 1 && limit <= int64(outboundapp.MaximumAudienceSendRecordLimit) && offset >= 0 && offset <= 100000
}

type audienceSendRecordResponse struct {
	RecordID                 string                         `json:"record_id"`
	Status                   outbound.CampaignDispatchState `json:"status"`
	TechnicalAttemptCount    int32                          `json:"technical_attempt_count"`
	FailureReason            string                         `json:"failure_reason,omitempty"`
	ProviderResultReceived   bool                           `json:"provider_result_received"`
	ReceiptPresent           bool                           `json:"receipt_present"`
	DeliveryProven           bool                           `json:"delivery_proven"`
	BusinessCallDispatched   bool                           `json:"business_call_dispatched"`
	RealExternalCallExecuted bool                           `json:"real_external_call_executed"`
	CreatedAt                time.Time                      `json:"created_at"`
	UpdatedAt                time.Time                      `json:"updated_at"`
}

func audienceSendRecordResponseOf(record outboundport.AudienceSendRecord) audienceSendRecordResponse {
	return audienceSendRecordResponse{
		RecordID: "campaign:" + strconv.FormatInt(record.ID, 10), Status: record.State,
		TechnicalAttemptCount: record.TechnicalAttemptCount, FailureReason: record.FailureClassification,
		ProviderResultReceived: record.ProviderResultReceived, ReceiptPresent: record.ReceiptPresent,
		DeliveryProven: record.DeliveryProven, BusinessCallDispatched: record.BusinessCallDispatched,
		RealExternalCallExecuted: record.RealExternalCallExecuted, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func writeAudienceSendRecordError(writer http.ResponseWriter, request *http.Request, err error) {
	var platformError *platformhttp.HTTPError
	if errors.As(err, &platformError) {
		platformhttp.WriteError(writer, request, platformError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, outbound.ErrCampaignDispatchInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, outbound.ErrCampaignHandoffNotFound):
		code = platformhttp.CodeNotFound
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}
