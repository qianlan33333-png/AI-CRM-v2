package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const ChannelAcquisitionQRCodeDownloadPath = ChannelAcquisitionAssetsRoutePrefix + "/{channel_id}/qrcode/download"

// ChannelAcquisitionQRCodeDownloadHandler is the only browser download path
// for a Provider QR image. It reads an executed CH02 asset then fetches and
// transcodes it server-side; the browser never follows the Provider asset URL.
type ChannelAcquisitionQRCodeDownloadHandler struct {
	queries     channelAcquisitionAssetQueries
	fetchClient *http.Client
	allowedHost string
}

func NewChannelAcquisitionQRCodeDownloadHandler(queries channelAcquisitionAssetQueries, fetchClient *http.Client, allowedHost string) (*ChannelAcquisitionQRCodeDownloadHandler, error) {
	if channelAcquisitionNil(queries) || fetchClient == nil || strings.TrimSpace(allowedHost) != allowedHost || allowedHost == "" {
		return nil, contactapp.ErrChannelAcquisitionAssetUnavailable
	}
	return &ChannelAcquisitionQRCodeDownloadHandler{queries: queries, fetchClient: fetchClient, allowedHost: allowedHost}, nil
}

func NewDisabledChannelAcquisitionQRCodeDownloadHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		channelAcquisitionWriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactapp.ErrChannelAcquisitionAssetUnavailable))
	})
}

func (handler *ChannelAcquisitionQRCodeDownloadHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	channelAcquisitionSecurityHeaders(writer)
	if handler == nil || request == nil || request.URL == nil || request.Method != http.MethodGet || request.URL.RawPath != "" || request.URL.RawQuery != "" || strings.HasSuffix(request.URL.Path, "/") || strings.Contains(request.URL.Path, "\\") {
		channelAcquisitionQRCodeDownloadError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	if !channelAcquisitionQRCodeReadAuthorized(request) {
		channelAcquisitionQRCodeDownloadError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(parts) != 6 || strings.Join(parts[:3], "/") != "api/admin/channels" || parts[4] != "qrcode" || parts[5] != "download" {
		channelAcquisitionQRCodeDownloadError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	channelID, err := channelAcquisitionID(parts[3])
	if err != nil {
		channelAcquisitionQRCodeDownloadError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	page, err := handler.queries.List(request.Context(), contactapp.ChannelAcquisitionAssetListInput{ChannelID: channelID, Limit: contactapp.ChannelAcquisitionAssetMaximumLimit})
	if err != nil {
		channelAcquisitionQRCodeDownloadError(writer, request, err)
		return
	}
	assetURL := ""
	for _, item := range page.Items {
		if item.Kind == contactport.AcquisitionAssetQRCode && item.State == eer.StateExecuted && item.AssetURL != "" {
			assetURL = item.AssetURL
			break
		}
	}
	if !handler.validAssetURL(assetURL) {
		channelAcquisitionQRCodeDownloadError(writer, request, contactapp.ErrChannelAcquisitionAssetNotFound)
		return
	}
	imageBytes, err := handler.fetchJPEG(request.Context(), assetURL)
	if err != nil {
		channelAcquisitionQRCodeDownloadError(writer, request, contactapp.ErrChannelAcquisitionAssetUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "image/jpeg")
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "channel-"+strconv.FormatInt(channelID, 10)+"-qrcode.jpg"))
	writer.Header().Set("Content-Length", strconv.Itoa(len(imageBytes)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(imageBytes)
}

func (handler *ChannelAcquisitionQRCodeDownloadHandler) validAssetURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host == handler.allowedHost && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && strings.TrimSpace(value) == value
}

func (handler *ChannelAcquisitionQRCodeDownloadHandler) fetchJPEG(ctx context.Context, assetURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "image/*")
	response, err := handler.fetchClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "image/") {
		return nil, errors.New("unexpected provider image response")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 5<<20+1))
	if err != nil || len(raw) == 0 || len(raw) > 5<<20 {
		return nil, errors.New("invalid provider image size")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err = jpeg.Encode(&output, decoded, &jpeg.Options{Quality: 90}); err != nil || output.Len() == 0 || output.Len() > 5<<20 {
		return nil, errors.New("jpeg conversion failed")
	}
	return output.Bytes(), nil
}

func channelAcquisitionQRCodeReadAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) && authorizationOK && authorization.Capability == authport.CapabilityChannelsRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func channelAcquisitionQRCodeDownloadError(writer http.ResponseWriter, request *http.Request, err error) {
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		channelAcquisitionWriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	if errors.Is(err, contactapp.ErrChannelAcquisitionAssetNotFound) {
		code = platformhttp.CodeNotFound
	}
	channelAcquisitionWriteError(writer, request, platformhttp.NewError(code, err))
}
