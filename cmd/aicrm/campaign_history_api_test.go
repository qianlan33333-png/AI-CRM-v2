package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

type campaignHistoryAPIReader struct {
	campaignport.CampaignHistoryReader
	calls                       int
	limit, offset               int32
	empty, invalid, wrongParent bool
	err                         error
}

func campaignAPIHistoricalCampaignSegment(id int64) campaignport.HistoricalCampaignSegment {
	return campaignport.HistoricalCampaignSegment{
		ID:                  id,
		SourceID:            11,
		CampaignSourceID:    11,
		SegmentSourceID:     11,
		SourceParentState:   "observed",
		Code:                "history",
		Priority:            11,
		Label:               "history",
		CreatedAt:           time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		SourcePayloadDigest: [32]byte{1},
	}
}

func campaignAPIHistoricalCampaignMember(id int64) campaignport.HistoricalCampaignMember {
	return campaignport.HistoricalCampaignMember{
		ID:                      id,
		SourceID:                11,
		CampaignSourceID:        11,
		CampaignSegmentSourceID: 11,
		SegmentSourceID:         11,
		MemberSourceID:          11,
		SegmentHistoryID:        11,
		JoinedAt:                time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		AnchorDate:              "history",
		CurrentStepIndex:        11,
		OriginalStatus:          "history",
		StopReason:              "history",
		RetryCount:              11,
		CreatedAt:               time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		UpdatedAt:               time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		SourcePayloadDigest:     [32]byte{1},
	}
}

func campaignAPIHistoricalBroadcastPlan(id int64) campaignport.HistoricalBroadcastPlan {
	return campaignport.HistoricalBroadcastPlan{
		ID:                    id,
		SourceID:              11,
		SourcePlanID:          "legacy-plan",
		DisplayName:           "history",
		Intent:                "history",
		ContentStrategy:       "history",
		ContentTemplateMasked: "history",
		MaxRecipients:         11,
		CandidateCount:        11,
		SkippedCount:          11,
		RequiresManualCopy:    true,
		OriginalStatus:        "history",
		OriginalReviewStatus:  "history",
		OriginalRunStatus:     "history",
		CreatedAt:             time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		UpdatedAt:             time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		RuntimeDigest:         [32]byte{1},
		SourcePayloadDigest:   [32]byte{1},
	}
}

func campaignAPIHistoricalBroadcastRecipient(id int64) campaignport.HistoricalBroadcastRecipient {
	return campaignport.HistoricalBroadcastRecipient{
		ID:                     id,
		SourceID:               11,
		PlanHistoryID:          11,
		DisplayName:            "history",
		PlannedMessageCount:    11,
		OriginalApprovalStatus: "history",
		OriginalSendStatus:     "history",
		CreatedAt:              time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		UpdatedAt:              time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		SourcePayloadDigest:    [32]byte{1},
	}
}

func campaignAPIHistoricalBroadcastMessage(id int64) campaignport.HistoricalBroadcastMessage {
	return campaignport.HistoricalBroadcastMessage{
		ID:                   id,
		SourceID:             11,
		PlanHistoryID:        11,
		RecipientHistoryID:   11,
		SequenceIndex:        11,
		DayOffset:            11,
		OriginalSendTime:     "09:00",
		ContentMasked:        "history",
		OriginalStatus:       "history",
		CreatedAt:            time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		UpdatedAt:            time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		ContentPayloadDigest: [32]byte{1},
		AttachmentsDigest:    [32]byte{1},
		SourcePayloadDigest:  [32]byte{1},
	}
}

func (r *campaignHistoryAPIReader) ListHistoricalCampaignSegments(_ context.Context, sourceID *int64, limit, offset int32) ([]campaignport.HistoricalCampaignSegment, int64, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	if r.empty {
		return nil, 0, r.err
	}
	row := campaignAPIHistoricalCampaignSegment(31)
	if sourceID != nil {
		row.CampaignSourceID = *sourceID
	}
	if r.wrongParent {
		row.CampaignSourceID = 999
	}
	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return []campaignport.HistoricalCampaignSegment{row}, 1, r.err
}

func (r *campaignHistoryAPIReader) ListHistoricalCampaignMembers(_ context.Context, segmentID, customerID *int64, limit, offset int32) ([]campaignport.HistoricalCampaignMember, int64, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	if r.empty {
		return nil, 0, r.err
	}
	row := campaignAPIHistoricalCampaignMember(31)
	if segmentID != nil {
		row.SegmentHistoryID = *segmentID
	}
	row.CustomerID = customerID
	if r.wrongParent {
		row.SegmentHistoryID = 999
	}
	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return []campaignport.HistoricalCampaignMember{row}, 1, r.err
}

func (r *campaignHistoryAPIReader) ListHistoricalBroadcastPlans(_ context.Context, limit, offset int32) ([]campaignport.HistoricalBroadcastPlan, int64, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	if r.empty {
		return nil, 0, r.err
	}
	row := campaignAPIHistoricalBroadcastPlan(31)

	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return []campaignport.HistoricalBroadcastPlan{row}, 1, r.err
}

func (r *campaignHistoryAPIReader) ListHistoricalBroadcastRecipients(_ context.Context, id int64, limit, offset int32) ([]campaignport.HistoricalBroadcastRecipient, int64, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	if r.empty {
		return nil, 0, r.err
	}
	row := campaignAPIHistoricalBroadcastRecipient(31)
	row.PlanHistoryID = id
	if r.wrongParent {
		row.PlanHistoryID = 999
	}
	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return []campaignport.HistoricalBroadcastRecipient{row}, 1, r.err
}

func (r *campaignHistoryAPIReader) ListHistoricalBroadcastMessages(_ context.Context, id int64, limit, offset int32) ([]campaignport.HistoricalBroadcastMessage, int64, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	if r.empty {
		return nil, 0, r.err
	}
	row := campaignAPIHistoricalBroadcastMessage(31)
	row.RecipientHistoryID = id
	if r.wrongParent {
		row.RecipientHistoryID = 999
	}
	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return []campaignport.HistoricalBroadcastMessage{row}, 1, r.err
}

func (r *campaignHistoryAPIReader) GetHistoricalCampaignSegment(_ context.Context, id int64) (campaignport.HistoricalCampaignSegment, error) {
	r.calls++
	row := campaignAPIHistoricalCampaignSegment(id)
	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return row, r.err
}

func (r *campaignHistoryAPIReader) GetHistoricalBroadcastPlan(_ context.Context, id int64) (campaignport.HistoricalBroadcastPlan, error) {
	r.calls++
	row := campaignAPIHistoricalBroadcastPlan(id)
	if r.invalid {
		row.SourcePayloadDigest = [32]byte{}
	}
	return row, r.err
}

func campaignHistoryRouter(t *testing.T, reader campaignport.CampaignHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &legacyAuthStub{principal: authport.Principal{AdminUserID: 1, Role: role}, csrfErr: authport.ErrUnauthorized}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.campaignHistory = reader
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

var campaignHistoryAPIPaths = []string{
	"/api/admin/campaign-history/segments",
	"/api/admin/campaign-history/segments/11",
	"/api/admin/campaign-history/members",
	"/api/admin/campaign-history/broadcast-plans",
	"/api/admin/campaign-history/broadcast-plans/11",
	"/api/admin/campaign-history/broadcast-plans/11/recipients",
	"/api/admin/campaign-history/broadcast-recipients/11/messages",
}

func TestFinalRouterCampaignHistoryAdminReadOnly(t *testing.T) {
	reader := &campaignHistoryAPIReader{}
	router := campaignHistoryRouter(t, reader, authport.RoleAdmin)
	for _, path := range campaignHistoryAPIPaths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xb1)))
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" ||
			!strings.Contains(response.Body.String(), `"read_only":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	if reader.calls != 7 || reader.limit != 50 || reader.offset != 0 {
		t.Fatalf("reader calls/page=%d/%d/%d", reader.calls, reader.limit, reader.offset)
	}
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		router = campaignHistoryRouter(t, reader, role)
		for _, path := range campaignHistoryAPIPaths {
			before := reader.calls
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			want := 401
			if role == authport.RoleOps {
				request = legacyRequest(http.MethodGet, path, legacyToken(0xb2))
				want = 403
			}
			router.ServeHTTP(response, request)
			if response.Code != want || reader.calls != before {
				t.Fatalf("role=%s %s status=%d", role, path, response.Code)
			}
		}
	}
	router = campaignHistoryRouter(t, reader, authport.RoleAdmin)
	for _, path := range campaignHistoryAPIPaths {
		before := reader.calls
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(0xb3)))
		if response.Code < 400 || reader.calls != before {
			t.Fatalf("POST reached reader: %s %d", path, response.Code)
		}
	}
}

func TestFinalRouterCampaignHistoryQueryAndFilters(t *testing.T) {
	reader := &campaignHistoryAPIReader{}
	router := campaignHistoryRouter(t, reader, authport.RoleAdmin)
	for _, path := range []string{
		"/api/admin/campaign-history/segments?campaign_source_id=11&limit=1&offset=0",
		"/api/admin/campaign-history/members?segment_history_id=11&customer_id=7&limit=1",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xb4)))
		if response.Code != 200 || reader.limit != 1 {
			t.Fatalf("filter GET %s %d", path, response.Code)
		}
		reader.wrongParent = true
		response = httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xb4)))
		if response.Code != 503 {
			t.Fatalf("wrong filter row accepted: %s %d", path, response.Code)
		}
		reader.wrongParent = false
	}
	for _, path := range []string{
		"/api/admin/campaign-history/segments?limit=0", "/api/admin/campaign-history/segments?limit=101",
		"/api/admin/campaign-history/segments?offset=-1", "/api/admin/campaign-history/segments?offset=2147483648",
		"/api/admin/campaign-history/segments?limit=01", "/api/admin/campaign-history/segments?limit=1&limit=2",
		"/api/admin/campaign-history/segments?unknown=1", "/api/admin/campaign-history/members?customer_id=0",
		"/api/admin/campaign-history/members?segment_history_id%20customer_id=1",
		"/api/admin/campaign-history/segments/01", "/api/admin/campaign-history/segments/11?limit=1",
		"/api/admin/campaign-history/broadcast-plans/0/recipients", "/api/admin/campaign-history/broadcast-recipients/01/messages",
	} {
		before := reader.calls
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xb5)))
		if response.Code != 400 || reader.calls != before {
			t.Fatalf("invalid GET %s: %d", path, response.Code)
		}
	}
}

func TestFinalRouterCampaignHistoryFailureAndEmpty(t *testing.T) {
	reader := &campaignHistoryAPIReader{}
	router := campaignHistoryRouter(t, reader, authport.RoleAdmin)
	for _, path := range campaignHistoryAPIPaths {
		for _, badDigest := range []bool{false, true} {
			reader.invalid = badDigest
			reader.err = nil
			if !badDigest {
				reader.err = errors.New("private database credentials")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xb6)))
			if response.Code != 503 || strings.Contains(response.Body.String(), "private database") || strings.Contains(response.Body.String(), `"items"`) {
				t.Fatalf("reader failure GET %s: %d %s", path, response.Code, response.Body.String())
			}
		}
	}
	reader.invalid, reader.err, reader.empty = false, nil, true
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, campaignHistoryAPIPaths[0], legacyToken(0xb7)))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("empty=%d %s", response.Code, response.Body.String())
	}
	var typedNil *campaignHistoryAPIReader
	router = campaignHistoryRouter(t, typedNil, authport.RoleAdmin)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, campaignHistoryAPIPaths[0], legacyToken(0xb8)))
	if response.Code != 503 {
		t.Fatalf("typed nil=%d", response.Code)
	}
}
