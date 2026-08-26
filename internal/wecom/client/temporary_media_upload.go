package client

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
type TemporaryMediaUploader struct {
	base  *url.URL
	http  *http.Client
	token TokenProvider
	now   func() time.Time
}

func NewTemporaryMediaUploader(baseURL string, httpClient *http.Client, token TokenProvider, now func() time.Time) (*TemporaryMediaUploader, error) {
	u, e := parseBaseURL(baseURL)
	if e != nil || httpClient == nil || token == nil || now == nil {
		return nil, ErrInvalidConfig
	}
	return &TemporaryMediaUploader{u, httpClient, token, now}, nil
}
func (u *TemporaryMediaUploader) Upload(ctx context.Context, in TemporaryMediaUpload) (TemporaryMediaResult, error) {
	if u == nil || ctx == nil || !validUpload(in) {
		return TemporaryMediaResult{}, ErrInvalidConfig
	}
	token, e := u.token.Token(ctx)
	if e != nil {
		return TemporaryMediaResult{}, e
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, e := form.CreatePart(textprotoMIMEHeader(in.Filename, in.MIME))
	if e != nil {
		return TemporaryMediaResult{}, e
	}
	if _, e = part.Write(in.Bytes); e != nil {
		return TemporaryMediaResult{}, e
	}
	_ = form.Close()
	endpoint := u.base.ResolveReference(&url.URL{Path: "/cgi-bin/media/upload"})
	q := endpoint.Query()
	q.Set("access_token", token.Value())
	q.Set("type", in.Kind)
	endpoint.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), &body)
	if e != nil {
		return TemporaryMediaResult{}, e
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, e := u.http.Do(req)
	if e != nil {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if e != nil || resp.StatusCode/100 != 2 {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, errors.Join(ErrTransport, e)
	}
	var out struct {
		ErrCode   int    `json:"errcode"`
		MediaID   string `json:"media_id"`
		CreatedAt int64  `json:"created_at"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, ErrUnexpectedResponse
	}
	if out.ErrCode != 0 {
		return TemporaryMediaResult{BusinessCallDispatched: true, FinalFailed: true}, ErrUpstream
	}
	if strings.TrimSpace(out.MediaID) != out.MediaID || out.MediaID == "" || out.CreatedAt < 1 {
		return TemporaryMediaResult{BusinessCallDispatched: true, OutcomeUnknown: true}, ErrUnexpectedResponse
	}
	created := time.Unix(out.CreatedAt, 0).UTC()
	return TemporaryMediaResult{MediaID: out.MediaID, CreatedAt: created, ExpiresAt: created.Add(71 * time.Hour), BusinessCallDispatched: true}, nil
}

func textprotoMIMEHeader(filename, mediaType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "media", "filename": filename}))
	header.Set("Content-Type", mediaType)
	return header
}
func validUpload(in TemporaryMediaUpload) bool {
	if (in.Kind != "image" && in.Kind != "file") || in.Filename == "" || in.MIME == "" || len(in.Bytes) == 0 || !strings.HasPrefix(in.Checksum, "sha256:") {
		return false
	}
	sum := sha256.Sum256(in.Bytes)
	return in.Checksum == "sha256:"+hex.EncodeToString(sum[:])
}
