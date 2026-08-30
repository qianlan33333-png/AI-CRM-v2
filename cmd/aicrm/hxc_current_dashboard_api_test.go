package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

type hxcCurrentDashboardStub struct {
	snapshot hxcport.DashboardSnapshot
	err      error
	calls    int
	limit    int32
}

func (stub *hxcCurrentDashboardStub) ReadDashboard(_ context.Context, limit int32) (hxcport.DashboardSnapshot, error) {
	stub.calls++
	stub.limit = limit
	return stub.snapshot, stub.err
}

func TestHXCCurrentDashboardReturnsSafeCurrentRows(t *testing.T) {
	stamp := time.Date(2026, 8, 30, 8, 9, 10, 0, time.UTC)
	lastCapability := "coach"
	reader := &hxcCurrentDashboardStub{snapshot: hxcport.DashboardSnapshot{
		Rows: []hxcport.DashboardRow{{
			HXCUserID: "private-user-1234", MatchState: hxcport.MatchStateUnmatched, SubscriptionTier: "member",
			CurrentPeriodUsed: 2, MonthlyChatQuota: 100, UserMessages7D: 3, UserMessages30D: 7,
			LastUsedAt: &stamp, LastCapability: &lastCapability, SourceUpdatedAt: stamp, SyncedAt: stamp,
		}},
		Total: 2859, UnmatchedCount: 2859, LastSyncedAt: &stamp,
	}}
	response := httptest.NewRecorder()
	(&Handler{hxcCurrentDashboard: reader}).ListHXCCurrentDashboard(response, httptest.NewRequest(http.MethodGet, hxcCurrentDashboardPath+"?limit=50", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || reader.calls != 1 || reader.limit != 50 {
		t.Fatalf("status=%d calls=%d limit=%d body=%s", response.Code, reader.calls, reader.limit, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"source":"hxc_current_sync"`, `"read_only":true`, `"real_external_call_executed":false`, `"total":2859`, `"user_ref":"HXC-****1234"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	if strings.Contains(body, "private-user-1234") {
		t.Fatalf("raw HXC user id leaked: %s", body)
	}
}

func TestHXCCurrentDashboardRejectsInvalidQueriesAndUnavailableReads(t *testing.T) {
	reader := &hxcCurrentDashboardStub{}
	for _, suffix := range []string{"?limit=0", "?limit=201", "?limit=1&limit=2", "?offset=1"} {
		response := httptest.NewRecorder()
		(&Handler{hxcCurrentDashboard: reader}).ListHXCCurrentDashboard(response, httptest.NewRequest(http.MethodGet, hxcCurrentDashboardPath+suffix, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("suffix=%s status=%d", suffix, response.Code)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("invalid query reached reader: %d", reader.calls)
	}
	for _, handler := range []*Handler{{}, {hxcCurrentDashboard: &hxcCurrentDashboardStub{err: errors.New("private database error")}}} {
		response := httptest.NewRecorder()
		handler.ListHXCCurrentDashboard(response, httptest.NewRequest(http.MethodGet, hxcCurrentDashboardPath, nil))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private database error") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
