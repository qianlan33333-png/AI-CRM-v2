package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

const maxWeComTemporaryMediaResponseBytes = 1 << 20

var (
	ErrInvalidWeComTemporaryMedia   = errors.New("invalid WeCom temporary media upload")
	ErrWeComTemporaryMediaTransport = errors.New("WeCom temporary media transport failure")
	ErrWeComTemporaryMediaResponse  = errors.New("invalid WeCom temporary media response")
	ErrWeComTemporaryMediaUpstream  = errors.New("WeCom temporary media API rejected request")
)

type TemporaryMediaUpload struct {
	Kind, Filename, MIME string
	Bytes                []byte
	Checksum             string
}

type TemporaryMediaResult struct {
	MediaID                                             string
	CreatedAt, ExpiresAt                                time.Time
	BusinessCallDispatched, OutcomeUnknown, FinalFailed bool
}

type TemporaryMediaTokenProvider interface {
	Token(context.Context) (string, error)
}

type TemporaryMediaUploader struct {
	base  *url.URL
	http  *http.Client
	token TemporaryMediaTokenProvider
	now   func() time.Time
}

func NewTemporaryMediaUploader(baseURL string, httpClient *http.Client, token TemporaryMediaTokenProvider, now func() time.Time) (*TemporaryMediaUploader, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || httpClient == nil || token == nil || now == nil {
		return nil, ErrInvalidWeComTemporaryMedia
	}
	return &TemporaryMediaUploader{base: parsed, http: httpClient, token: token, now: now}, nil
}

func (uploader *TemporaryMediaUploader) Upload(ctx context.Context, input TemporaryMediaUpload) (TemporaryMediaResult, error) {
	if uploader == nil || ctx == nil || !validTemporaryMediaUpload(input) {
		return TemporaryMediaResult{}, ErrInvalidWeComTemporaryMedia
	}
	token, err := uploader.token.Token(ctx)
	if err != nil || token == "" {
		return TemporaryMediaResult{}, errors.Join(ErrWeComTemporaryMediaTransport, err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreatePart(temporaryMediaMIMEHeader(input.Filename, input.MIME))
	if err != nil {
		return TemporaryMediaResult{}, err
	}
	if _, err = part.Write(input.Bytes); err != nil {
		return TemporaryMediaResult{}, err
	}
	if err = form.Close(); err != nil {
		return TemporaryMediaResult{}, err
	}
	endpoint := uploader.base.ResolveReference(&url.URL{Path: "/cgi-bin/media/upload"})
	query := endpoint.Query()
	query.Set("access_token", token)
	query.Set("type", input.Kind)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), &body)
	if err != nil {
		return TemporaryMediaResult{}, err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := uploader.http.Do(request)
	if err != nil {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxWeComTemporaryMediaResponseBytes+1))
	if err != nil || len(raw) > maxWeComTemporaryMediaResponseBytes || response.StatusCode/100 != 2 {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, errors.Join(ErrWeComTemporaryMediaTransport, err)
	}
	var result struct {
		ErrCode   int    `json:"errcode"`
		MediaID   string `json:"media_id"`
		CreatedAt int64  `json:"created_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&result) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, ErrWeComTemporaryMediaResponse
	}
	if result.ErrCode != 0 {
		return TemporaryMediaResult{BusinessCallDispatched: true, FinalFailed: true}, ErrWeComTemporaryMediaUpstream
	}
	if strings.TrimSpace(result.MediaID) != result.MediaID || result.MediaID == "" || result.CreatedAt < 1 {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, ErrWeComTemporaryMediaResponse
	}
	createdAt := time.Unix(result.CreatedAt, 0).UTC()
	return TemporaryMediaResult{MediaID: result.MediaID, CreatedAt: createdAt, ExpiresAt: createdAt.Add(71 * time.Hour), BusinessCallDispatched: true}, nil
}

func temporaryMediaMIMEHeader(filename, mediaType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "media", "filename": filename}))
	header.Set("Content-Type", mediaType)
	return header
}

func validTemporaryMediaUpload(input TemporaryMediaUpload) bool {
	if (input.Kind != "image" && input.Kind != "file") || input.Filename == "" || input.MIME == "" || len(input.Bytes) == 0 || !strings.HasPrefix(input.Checksum, "sha256:") {
		return false
	}
	sum := sha256.Sum256(input.Bytes)
	return input.Checksum == "sha256:"+hex.EncodeToString(sum[:])
}
