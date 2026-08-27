package membergrid

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type publicShareApplicationStub struct {
	result PublicShareSummary
	err    error
	token  string
	cursor string
	calls  int
}

func (application *publicShareApplicationStub) Summary(_ context.Context, token, cursor string) (PublicShareSummary, error) {
	application.calls++
	application.token = token
	application.cursor = cursor
	return application.result, application.err
}

func TestPublicShareHTTPReturnsSafeGridAndNoStore(t *testing.T) {
	application := &publicShareApplicationStub{result: PublicShareSummary{
		Buckets: []PublicShareBucket{{State: "active", Count: 3}, {State: "expired", Count: 1}, {State: "removed", Count: 0}},
		Rows:    []PublicShareMember{{DisplayName: "李同学", State: "active", Source: "manual", StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)}},
		Limit:   50,
		AsOf:    time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC),
	}}
	handler, err := NewPublicShareHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, PublicShareSummaryPath, strings.NewReader(`{"token":"mgshare1.share_abcdefghijklmnopqrstuv.signature","cursor":"mg2.opaque"}`)))
	if response.Code != http.StatusOK || application.calls != 1 || application.token == "" || application.cursor != "mg2.opaque" {
		t.Fatalf("status/calls/token=%d/%d/%q body=%s", response.Code, application.calls, application.token, response.Body.String())
	}
	for key, value := range map[string]string{"Cache-Control": "private, no-store", "Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff"} {
		if response.Header().Get(key) != value {
			t.Fatalf("%s=%q", key, response.Header().Get(key))
		}
	}
	body := response.Body.String()
	for _, forbidden := range []string{"customer_id", "member_ref", "service_product_id", "mobile", "unionid", "external_userid", "remark", "alliance", "share_abcdefghijklmnopqrstuv"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public body leaked %s: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "李同学") {
		t.Fatalf("public body omitted allowlisted member row: %s", body)
	}
}

func TestPublicShareHTTPFailsClosed(t *testing.T) {
	for name, testCase := range map[string]struct {
		method string
		path   string
		body   string
		err    error
		want   int
	}{
		"invalid json":  {method: http.MethodPost, path: PublicShareSummaryPath, body: `{`, want: http.StatusBadRequest},
		"unknown field": {method: http.MethodPost, path: PublicShareSummaryPath, body: `{"token":"x","extra":true}`, want: http.StatusBadRequest},
		"long cursor":   {method: http.MethodPost, path: PublicShareSummaryPath, body: `{"token":"x","cursor":"` + strings.Repeat("x", 257) + `"}`, want: http.StatusBadRequest},
		"bad cursor":    {method: http.MethodPost, path: PublicShareSummaryPath, body: `{"token":"x","cursor":"bad"}`, err: ErrInvalidCursor, want: http.StatusBadRequest},
		"query":         {method: http.MethodPost, path: PublicShareSummaryPath + "?token=x", body: `{"token":"x"}`, want: http.StatusNotFound},
		"disabled":      {method: http.MethodPost, path: PublicShareSummaryPath, body: `{"token":"x"}`, err: ErrNotFound, want: http.StatusNotFound},
		"unavailable":   {method: http.MethodPost, path: PublicShareSummaryPath, body: `{"token":"x"}`, err: errors.New("database"), want: http.StatusServiceUnavailable},
		"method":        {method: http.MethodGet, path: PublicShareSummaryPath, want: http.StatusMethodNotAllowed},
	} {
		t.Run(name, func(t *testing.T) {
			application := &publicShareApplicationStub{err: testCase.err}
			handler, err := NewPublicShareHandler(application)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body)))
			if response.Code != testCase.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, testCase.want, response.Body.String())
			}
		})
	}
}
