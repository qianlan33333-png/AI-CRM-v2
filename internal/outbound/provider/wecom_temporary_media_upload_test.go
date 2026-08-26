package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTemporaryMediaUploadUsesExactMultipartContract(t *testing.T) {
	content := []byte("image-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/cgi-bin/media/upload" || request.URL.Query().Get("access_token") != "token-safe" || request.URL.Query().Get("type") != "image" {
			t.Fatalf("request=%s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if err := request.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		file, header, err := request.FormFile("media")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		got, err := io.ReadAll(file)
		if err != nil || string(got) != string(content) || header.Filename != "cover.png" || header.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("body=%q header=%+v err=%v", got, header.Header, err)
		}
		_, _ = writer.Write([]byte(`{"errcode":0,"media_id":"media-1","created_at":1780000000}`))
	}))
	defer server.Close()
	uploader, err := NewTemporaryMediaUploader(server.URL, server.Client(), temporaryMediaTokenStub("token-safe"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := uploader.Upload(context.Background(), temporaryMediaUploadInput("image", "cover.png", "image/png", content))
	if err != nil || !result.BusinessCallDispatched || result.OutcomeUnknown || result.FinalFailed || result.MediaID != "media-1" || !result.ExpiresAt.Equal(time.Unix(1780000000, 0).UTC().Add(71*time.Hour)) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestTemporaryMediaUploadClassifiesFailuresWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name        string
		body        string
		status      int
		wantUnknown bool
		wantFinal   bool
		wantErr     error
	}{
		{name: "provider rejection", body: `{"errcode":40058}`, status: http.StatusOK, wantFinal: true, wantErr: ErrWeComTemporaryMediaUpstream},
		{name: "invalid response", body: `not-json`, status: http.StatusOK, wantUnknown: true, wantErr: ErrWeComTemporaryMediaResponse},
		{name: "non-2xx", body: `gateway`, status: http.StatusBadGateway, wantUnknown: true, wantErr: ErrWeComTemporaryMediaTransport},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls++
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			uploader, err := NewTemporaryMediaUploader(server.URL, server.Client(), temporaryMediaTokenStub("token-not-in-errors"), time.Now)
			if err != nil {
				t.Fatal(err)
			}
			result, err := uploader.Upload(context.Background(), temporaryMediaUploadInput("file", "readme.txt", "text/plain", []byte("one")))
			if calls != 1 || !errors.Is(err, test.wantErr) || !result.BusinessCallDispatched || result.OutcomeUnknown != test.wantUnknown || result.FinalFailed != test.wantFinal || strings.Contains(err.Error(), "token-not-in-errors") {
				t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
			}
		})
	}
}

func TestTemporaryMediaUploadRejectsInvalidChecksumBeforeDispatch(t *testing.T) {
	uploader, err := NewTemporaryMediaUploader("https://wecom.invalid", &http.Client{}, temporaryMediaTokenStub("token-safe"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := uploader.Upload(context.Background(), TemporaryMediaUpload{Kind: "file", Filename: "a.txt", MIME: "text/plain", Bytes: []byte("a"), Checksum: "sha256:bad"})
	if !errors.Is(err, ErrInvalidWeComTemporaryMedia) || result.BusinessCallDispatched {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type temporaryMediaTokenStub string

func (stub temporaryMediaTokenStub) Token(context.Context) (string, error) { return string(stub), nil }

func temporaryMediaUploadInput(kind, filename, mediaType string, content []byte) TemporaryMediaUpload {
	sum := sha256.Sum256(content)
	return TemporaryMediaUpload{Kind: kind, Filename: filename, MIME: mediaType, Bytes: content, Checksum: "sha256:" + hex.EncodeToString(sum[:])}
}
