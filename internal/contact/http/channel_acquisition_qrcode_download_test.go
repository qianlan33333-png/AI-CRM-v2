package http

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

func TestCH02QRCodeDownloadFetchesExecutedAssetServerSideAndTranscodesJPEG(t *testing.T) {
	var pngBody bytes.Buffer
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if err := png.Encode(&pngBody, pixel); err != nil {
		t.Fatal(err)
	}
	queries := &acquisitionAssetQueryStub{page: contactapp.ChannelAcquisitionAssetPage{Items: []contactapp.ChannelAcquisitionAssetItem{{ChannelID: 41, Kind: contactport.AcquisitionAssetQRCode, State: eer.StateExecuted, AssetURL: "https://work.weixin.qq.com/ca/qrcode"}}}}
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://work.weixin.qq.com/ca/qrcode" || request.Header.Get("Accept") != "image/*" {
			t.Fatalf("request=%s accept=%q", request.URL, request.Header.Get("Accept"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(pngBody.Bytes())), Request: request}, nil
	})}
	handler, err := NewChannelAcquisitionQRCodeDownloadHandler(queries, client, "work.weixin.qq.com")
	if err != nil {
		t.Fatal(err)
	}
	request := acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/qrcode/download", "", authport.CapabilityChannelsRead)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" || !bytes.HasPrefix(response.Body.Bytes(), []byte{0xff, 0xd8}) || queries.calls != 1 {
		t.Fatalf("status/headers/body/calls=%d/%q/%x/%d", response.Code, response.Header().Get("Content-Type"), response.Body.Bytes(), queries.calls)
	}
}

func TestCH02QRCodeDownloadRejectsProviderURLAndUnauthorizedRead(t *testing.T) {
	queries := &acquisitionAssetQueryStub{page: contactapp.ChannelAcquisitionAssetPage{Items: []contactapp.ChannelAcquisitionAssetItem{{ChannelID: 41, Kind: contactport.AcquisitionAssetQRCode, State: eer.StateExecuted, AssetURL: "https://evil.example/qrcode"}}}}
	handler, err := NewChannelAcquisitionQRCodeDownloadHandler(queries, http.DefaultClient, "work.weixin.qq.com")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/qrcode/download", "", authport.CapabilityChannelsRead))
	if response.Code != http.StatusNotFound {
		t.Fatalf("provider URL status/body=%d/%s", response.Code, response.Body.String())
	}
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, acquisitionAssetRequest(http.MethodGet, "/api/admin/channels/41/qrcode/download", "", authport.CapabilityChannelsWrite))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("authorization status/body=%d/%s", denied.Code, denied.Body.String())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
