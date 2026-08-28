package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type invalidSourceHistoryAPIReader struct {
	err   error
	calls int
	tag   contactport.HistoricalUnboundTag
	chan_ contactport.HistoricalInvalidChannel
	asset mediaport.HistoricalInvalidAsset
	radar radarport.HistoricalInvalidRadarLink
}

func (reader *invalidSourceHistoryAPIReader) GetHistoricalUnboundTag(_ context.Context, id int64) (contactport.HistoricalUnboundTag, error) {
	reader.calls++
	value := reader.tag
	value.ID = id
	return value, reader.err
}
func (reader *invalidSourceHistoryAPIReader) ListHistoricalUnboundTag(_ context.Context, _ contactport.InvalidSourceHistoryQuery) ([]contactport.HistoricalUnboundTag, int64, error) {
	reader.calls++
	return []contactport.HistoricalUnboundTag{reader.tag}, 1, reader.err
}
func (reader *invalidSourceHistoryAPIReader) GetHistoricalInvalidChannel(_ context.Context, id int64) (contactport.HistoricalInvalidChannel, error) {
	reader.calls++
	value := reader.chan_
	value.ID = id
	return value, reader.err
}
func (reader *invalidSourceHistoryAPIReader) ListHistoricalInvalidChannel(_ context.Context, _ contactport.InvalidSourceHistoryQuery) ([]contactport.HistoricalInvalidChannel, int64, error) {
	reader.calls++
	return []contactport.HistoricalInvalidChannel{reader.chan_}, 1, reader.err
}
func (reader *invalidSourceHistoryAPIReader) GetHistoricalInvalidAsset(_ context.Context, id int64) (mediaport.HistoricalInvalidAsset, error) {
	reader.calls++
	value := reader.asset
	value.ID = id
	return value, reader.err
}
func (reader *invalidSourceHistoryAPIReader) ListHistoricalInvalidAsset(_ context.Context, _ mediaport.InvalidSourceHistoryQuery) ([]mediaport.HistoricalInvalidAsset, int64, error) {
	reader.calls++
	return []mediaport.HistoricalInvalidAsset{reader.asset}, 1, reader.err
}
func (reader *invalidSourceHistoryAPIReader) GetHistoricalInvalidRadarLink(_ context.Context, id int64) (radarport.HistoricalInvalidRadarLink, error) {
	reader.calls++
	value := reader.radar
	value.ID = id
	return value, reader.err
}
func (reader *invalidSourceHistoryAPIReader) ListHistoricalInvalidRadarLink(_ context.Context, _ radarport.InvalidSourceHistoryQuery) ([]radarport.HistoricalInvalidRadarLink, int64, error) {
	reader.calls++
	return []radarport.HistoricalInvalidRadarLink{reader.radar}, 1, reader.err
}

func invalidSourceHistoryAPIDigest(seed byte) [32]byte {
	var digest [32]byte
	for index := range digest {
		digest[index] = seed
	}
	return digest
}

func invalidSourceHistoryAPIFixture() *invalidSourceHistoryAPIReader {
	stamp := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	return &invalidSourceHistoryAPIReader{
		tag: contactport.HistoricalUnboundTag{
			ID: 11, SourceKeyDigest: invalidSourceHistoryAPIDigest(1), SourcePayloadDigest: invalidSourceHistoryAPIDigest(2), SourceFieldDigest: invalidSourceHistoryAPIDigest(3), PrivateDigest: invalidSourceHistoryAPIDigest(4), RedactedRoots: []string{"private-root"}, TagSourceID: "", UnionIDDigest: invalidSourceHistoryAPIDigest(5), CreatedAt: stamp, QuarantineReason: "invalid_contact_tag",
		},
		chan_: contactport.HistoricalInvalidChannel{
			ID: 12, SourceKeyDigest: invalidSourceHistoryAPIDigest(6), SourcePayloadDigest: invalidSourceHistoryAPIDigest(7), SourceFieldDigest: invalidSourceHistoryAPIDigest(8), PrivateDigest: invalidSourceHistoryAPIDigest(9), RedactedRoots: []string{"private-root"}, SourceID: -12, Code: "", Name: "legacy channel", ChannelType: "qrcode", CarrierType: "qrcode", CreatedAt: stamp, UpdatedAt: stamp.Add(time.Second), QuarantineReason: "invalid_channel_definition",
		},
		asset: mediaport.HistoricalInvalidAsset{
			ID: 13, SourceKeyDigest: invalidSourceHistoryAPIDigest(10), SourcePayloadDigest: invalidSourceHistoryAPIDigest(11), SourceFieldDigest: invalidSourceHistoryAPIDigest(12), PrivateDigest: invalidSourceHistoryAPIDigest(13), RedactedRoots: []string{"private-root"}, Kind: "attachment", SourceID: -13, Name: "", FileName: "legacy.pdf", MIMEType: "application/pdf", FileSize: -14, OriginalEnabled: false, ContentDigest: invalidSourceHistoryAPIDigest(14), CreatedAt: stamp, UpdatedAt: stamp.Add(2 * time.Second), QuarantineReason: "invalid_static_media_definition",
		},
		radar: radarport.HistoricalInvalidRadarLink{
			ID: 14, SourceKeyDigest: invalidSourceHistoryAPIDigest(15), SourcePayloadDigest: invalidSourceHistoryAPIDigest(16), SourceFieldDigest: invalidSourceHistoryAPIDigest(17), PrivateDigest: invalidSourceHistoryAPIDigest(18), RedactedRoots: []string{"private-root"}, SourceID: -14, Code: "", Title: "legacy radar", DestinationURLDigest: invalidSourceHistoryAPIDigest(19), CreatedAt: stamp, UpdatedAt: stamp.Add(3 * time.Second), QuarantineReason: "invalid_radar_definition",
		},
	}
}

func invalidSourceHistoryAPIRouter(t *testing.T, contact contactport.InvalidSourceHistoryReader, media mediaport.InvalidSourceHistoryReader, radar radarport.InvalidSourceHistoryReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.contactInvalidSourceHistory = contact
	legacy.mediaInvalidSourceHistory = media
	legacy.radarInvalidSourceHistory = radar
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

func invalidSourceHistoryAPIResponse(t *testing.T, response *httptest.ResponseRecorder, detail bool) map[string]any {
	t.Helper()
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["source"] != "v1_history" || body["read_only"] != true || body["real_external_call_executed"] != false {
		t.Fatalf("history boundary=%v", body)
	}
	if detail {
		item, ok := body["item"].(map[string]any)
		if !ok {
			t.Fatalf("missing detail item=%v", body)
		}
		return item
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 || body["total"] != float64(1) || body["limit"] != float64(1) || body["offset"] != float64(0) {
		t.Fatalf("invalid list=%v", body)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid list item=%v", body)
	}
	return item
}

func TestFinalRouterInvalidSourceHistoryAdminReadOnly(t *testing.T) {
	stamp := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	for _, test := range []struct {
		name, list, detail string
		id                 int64
		want               map[string]any
	}{
		{"unbound_tag", "/api/admin/contact-invalid-history/tags", "/api/admin/contact-invalid-history/tags/11", 11, map[string]any{"id": float64(11), "tag_source_id": "", "created_at": stamp.Format(time.RFC3339Nano), "quarantine_reason": "invalid_contact_tag"}},
		{"invalid_channel", "/api/admin/contact-invalid-history/channels", "/api/admin/contact-invalid-history/channels/12", 12, map[string]any{"id": float64(12), "source_id": float64(-12), "code": "", "name": "legacy channel", "channel_type": "qrcode", "carrier_type": "qrcode", "created_at": stamp.Format(time.RFC3339Nano), "updated_at": stamp.Add(time.Second).Format(time.RFC3339Nano), "quarantine_reason": "invalid_channel_definition"}},
		{"invalid_asset", "/api/admin/media-invalid-history", "/api/admin/media-invalid-history/13", 13, map[string]any{"id": float64(13), "kind": "attachment", "source_id": float64(-13), "name": "", "file_name": "legacy.pdf", "mime_type": "application/pdf", "file_size": float64(-14), "original_enabled": false, "created_at": stamp.Format(time.RFC3339Nano), "updated_at": stamp.Add(2 * time.Second).Format(time.RFC3339Nano), "quarantine_reason": "invalid_static_media_definition"}},
		{"invalid_radar_link", "/api/admin/radar-invalid-history", "/api/admin/radar-invalid-history/14", 14, map[string]any{"id": float64(14), "source_id": float64(-14), "code": "", "title": "legacy radar", "created_at": stamp.Format(time.RFC3339Nano), "updated_at": stamp.Add(3 * time.Second).Format(time.RFC3339Nano), "quarantine_reason": "invalid_radar_definition"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, auth := invalidSourceHistoryAPIFixture(), &audienceHistoryAPIAuth{role: authport.RoleAdmin}
			router := invalidSourceHistoryAPIRouter(t, reader, reader, reader, auth)
			for detail, path := range map[bool]string{false: test.list + "?limit=1&offset=0", true: test.detail} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xd1)))
				item := invalidSourceHistoryAPIResponse(t, response, detail)
				if !reflect.DeepEqual(item, test.want) {
					t.Fatalf("public record=%v want=%v", item, test.want)
				}
				for _, private := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "private_digest", "redacted_roots", "union_id_digest", "content_digest", "destination_url_digest", "private-root"} {
					if _, found := item[private]; found || strings.Contains(response.Body.String(), private) {
						t.Fatalf("private value leaked=%s body=%s", private, response.Body.String())
					}
				}
			}
			if reader.calls != 2 || auth.csrfCalls != 0 || len(auth.capabilities) != 2 {
				t.Fatalf("reader/auth calls=%d csrf=%d capabilities=%v", reader.calls, auth.csrfCalls, auth.capabilities)
			}
			for _, capability := range auth.capabilities {
				if capability != authport.CapabilityAdminRead {
					t.Fatalf("capability=%s", capability)
				}
			}
		})
	}
}

func TestFinalRouterInvalidSourceHistoryRejectsUnauthorizedInvalidAndWriteRoutes(t *testing.T) {
	routes := []struct{ list, detail string }{
		{"/api/admin/contact-invalid-history/tags", "/api/admin/contact-invalid-history/tags/11"},
		{"/api/admin/contact-invalid-history/channels", "/api/admin/contact-invalid-history/channels/12"},
		{"/api/admin/media-invalid-history", "/api/admin/media-invalid-history/13"},
		{"/api/admin/radar-invalid-history", "/api/admin/radar-invalid-history/14"},
	}
	for _, test := range []struct {
		name string
		role authport.Role
		want int
	}{{"anonymous", "", http.StatusUnauthorized}, {"ops", authport.RoleOps, http.StatusForbidden}} {
		t.Run(test.name, func(t *testing.T) {
			reader := invalidSourceHistoryAPIFixture()
			router := invalidSourceHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: test.role})
			for _, route := range routes {
				for _, path := range []string{route.list, route.detail} {
					request := httptest.NewRequest(http.MethodGet, path, nil)
					if test.role != "" {
						request = legacyRequest(http.MethodGet, path, legacyToken(0xd2))
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != test.want {
						t.Fatalf("%s status=%d", path, response.Code)
					}
				}
			}
			if reader.calls != 0 {
				t.Fatalf("denied request reached reader calls=%d", reader.calls)
			}
		})
	}
	for _, route := range routes {
		reader := invalidSourceHistoryAPIFixture()
		router := invalidSourceHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
		for _, query := range []string{"limit=0", "limit=101", "limit=1&limit=2", "offset=-1", "offset=1&offset=2", "unknown=true", "limit=%zz"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, route.list+"?"+query, legacyToken(0xd3)))
			if response.Code != http.StatusBadRequest || reader.calls != 0 {
				t.Fatalf("%s?%s status=%d calls=%d", route.list, query, response.Code, reader.calls)
			}
		}
		for _, id := range []string{"0", "01", "-1", "x", "9223372036854775808"} {
			path := route.list + "/" + id
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xd4)))
			if response.Code != http.StatusBadRequest || reader.calls != 0 {
				t.Fatalf("%s status=%d calls=%d", path, response.Code, reader.calls)
			}
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, route.detail+"?limit=1", legacyToken(0xd5)))
		if response.Code != http.StatusBadRequest || reader.calls != 0 {
			t.Fatalf("detail query status=%d calls=%d", response.Code, reader.calls)
		}
		for _, path := range []string{route.list, route.detail} {
			response = httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(0xd6)))
			if response.Code >= http.StatusOK && response.Code < http.StatusMultipleChoices || reader.calls != 0 {
				t.Fatalf("write route=%s status=%d calls=%d", path, response.Code, reader.calls)
			}
		}
	}
}

func TestFinalRouterInvalidSourceHistoryFailsClosed(t *testing.T) {
	for _, route := range []struct {
		name, path string
		contact    contactport.InvalidSourceHistoryReader
		media      mediaport.InvalidSourceHistoryReader
		radar      radarport.InvalidSourceHistoryReader
	}{
		{"contact_nil", "/api/admin/contact-invalid-history/tags", nil, invalidSourceHistoryAPIFixture(), invalidSourceHistoryAPIFixture()},
		{"contact_typed_nil", "/api/admin/contact-invalid-history/channels", (*invalidSourceHistoryAPIReader)(nil), invalidSourceHistoryAPIFixture(), invalidSourceHistoryAPIFixture()},
		{"media_nil", "/api/admin/media-invalid-history", invalidSourceHistoryAPIFixture(), nil, invalidSourceHistoryAPIFixture()},
		{"radar_typed_nil", "/api/admin/radar-invalid-history", invalidSourceHistoryAPIFixture(), invalidSourceHistoryAPIFixture(), (*invalidSourceHistoryAPIReader)(nil)},
	} {
		t.Run(route.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			invalidSourceHistoryAPIRouter(t, route.contact, route.media, route.radar, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, route.path, legacyToken(0xd7)))
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "source_key_digest") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	for _, route := range []string{"/api/admin/contact-invalid-history/tags", "/api/admin/contact-invalid-history/channels", "/api/admin/media-invalid-history", "/api/admin/radar-invalid-history"} {
		reader := invalidSourceHistoryAPIFixture()
		reader.err = errors.New("private reader failure")
		response := httptest.NewRecorder()
		invalidSourceHistoryAPIRouter(t, reader, reader, reader, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, route, legacyToken(0xd8)))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private reader failure") {
			t.Fatalf("route=%s status=%d body=%s", route, response.Code, response.Body.String())
		}
	}
}

var _ contactport.InvalidSourceHistoryReader = (*invalidSourceHistoryAPIReader)(nil)
var _ mediaport.InvalidSourceHistoryReader = (*invalidSourceHistoryAPIReader)(nil)
var _ radarport.InvalidSourceHistoryReader = (*invalidSourceHistoryAPIReader)(nil)
