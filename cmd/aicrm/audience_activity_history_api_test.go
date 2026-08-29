package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestAudienceActivityHistoryAPIIsReadOnlyAndFailsClosed(t *testing.T) {
	stamp := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	handler := &Handler{audienceActivityHistory: audienceActivityHistoryReader{runs: []segmentport.AudienceActivityRunView{{ID: 8, PackageHistoryID: 4, RunType: "refresh", OriginalStatus: "done", RefreshStartedAt: stamp, CreatedAt: stamp}}, events: []segmentport.AudienceActivityMemberEventView{{ID: 9, PackageHistoryID: 4, EventType: "entered", OccurredAt: stamp, CreatedAt: stamp}}}}
	for _, test := range []struct {
		path string
		call func(http.ResponseWriter, *http.Request)
		want string
	}{
		{"/api/admin/audience-history/activity-runs?limit=1&offset=0", handler.ListAudienceActivityHistoryRuns, `"run_type":"refresh"`},
		{"/api/admin/audience-history/activity-member-events?limit=1&offset=0", handler.ListAudienceActivityHistoryMemberEvents, `"event_type":"entered"`},
	} {
		recorder := httptest.NewRecorder()
		test.call(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		body := recorder.Body.String()
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, test.want) || strings.Contains(body, "source_key_digest") || strings.Contains(body, "private_digest") || strings.Contains(body, "identity_kind") {
			t.Fatalf("path=%s status=%d body=%s", test.path, recorder.Code, body)
		}
	}

	for _, path := range []string{"/api/admin/audience-history/activity-runs?limit=0", "/api/admin/audience-history/activity-member-events?unknown=1"} {
		recorder := httptest.NewRecorder()
		if strings.Contains(path, "runs") {
			handler.ListAudienceActivityHistoryRuns(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		} else {
			handler.ListAudienceActivityHistoryMemberEvents(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid query accepted: path=%s status=%d", path, recorder.Code)
		}
	}

	handler.audienceActivityHistory = audienceActivityHistoryReader{err: errors.New("downstream")}
	recorder := httptest.NewRecorder()
	handler.ListAudienceActivityHistoryRuns(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/audience-history/activity-runs", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("downstream failure=%d", recorder.Code)
	}
}

type audienceActivityHistoryReader struct {
	runs   []segmentport.AudienceActivityRunView
	events []segmentport.AudienceActivityMemberEventView
	err    error
}

func (reader audienceActivityHistoryReader) ListAudienceActivityRuns(_ context.Context, _ int64, _ int32, _ int32) ([]segmentport.AudienceActivityRunView, int64, error) {
	return reader.runs, int64(len(reader.runs)), reader.err
}

func (reader audienceActivityHistoryReader) ListAudienceActivityMemberEvents(_ context.Context, _ int64, _ int32, _ int32) ([]segmentport.AudienceActivityMemberEventView, int64, error) {
	return reader.events, int64(len(reader.events)), reader.err
}
