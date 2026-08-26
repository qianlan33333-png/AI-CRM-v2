package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const maxWeComContactProfileResponseBytes = 1 << 20

var ErrInvalidWeComContactProfileProvider = errors.New("invalid WeCom contact profile provider")

type ContactProfileTokenProvider interface {
	Token(context.Context) (string, error)
	RefreshToken(context.Context) (string, error)
}
type WeComContactProfileClientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      ContactProfileTokenProvider
}
type WeComContactProfileClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      ContactProfileTokenProvider
}

func NewWeComContactProfileClient(c WeComContactProfileClientConfig) (*WeComContactProfileClient, error) {
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || c.HTTPClient == nil || c.Token == nil {
		return nil, ErrInvalidWeComContactProfileProvider
	}
	return &WeComContactProfileClient{baseURL: u, httpClient: c.HTTPClient, token: c.Token}, nil
}

func (c *WeComContactProfileClient) WriteContactProfile(ctx context.Context, r wecomport.ContactProfileWriteRequest) (eer.AdapterResult, error) {
	if c == nil || ctx == nil || c.httpClient == nil || c.token == nil || !validContactProfileRequest(r) {
		return finalProfileResult("invalid", false, false), nil
	}
	token, err := c.token.Token(ctx)
	if err != nil || !validProfileToken(token) {
		return finalProfileResult("token", false, false), nil
	}
	result, expired := c.writeWithToken(ctx, token, r)
	if !expired {
		return result, nil
	}
	token, err = c.token.RefreshToken(ctx)
	if err != nil || !validProfileToken(token) {
		return result, nil
	}
	result, _ = c.writeWithToken(ctx, token, r)
	return result, nil
}
func (c *WeComContactProfileClient) writeWithToken(ctx context.Context, token string, r wecomport.ContactProfileWriteRequest) (eer.AdapterResult, bool) {
	body, _ := json.Marshal(map[string]string{"userid": r.StaffUserID, "external_userid": r.ExternalUserID, "remark": r.Remark, "description": r.Description})
	u := c.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/externalcontact/remark"})
	q := url.Values{}
	q.Set("access_token", token)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return finalProfileResult("request", false, false), false
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return unknownProfileResult("transport"), false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return unknownProfileResult("http-" + strconv.Itoa(response.StatusCode)), false
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWeComContactProfileResponseBytes+1))
	if err != nil || len(data) > maxWeComContactProfileResponseBytes {
		return unknownProfileResult("body"), false
	}
	var payload struct {
		ErrCode int `json:"errcode"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&payload) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return unknownProfileResult("json"), false
	}
	if payload.ErrCode == 0 {
		return executedProfileResult("ok"), false
	}
	if payload.ErrCode == 40014 || payload.ErrCode == 42001 {
		return finalProfileResult("expired-"+strconv.Itoa(payload.ErrCode), true, true), true
	}
	return finalProfileResult("upstream-"+strconv.Itoa(payload.ErrCode), true, true), false
}
func validContactProfileRequest(r wecomport.ContactProfileWriteRequest) bool {
	return validProfileText(r.CorpID, 1, 256) && validProfileText(r.StaffUserID, 1, 128) && validProfileText(r.ExternalUserID, 1, 1024) && validProfileText(r.Remark, 1, 400) && validOptionalProfileText(r.Description, 1500)
}
func validProfileToken(v string) bool {
	return validProfileText(v, 1, 4096) && strings.IndexFunc(v, unicode.IsSpace) < 0
}
func validProfileText(v string, min, max int) bool {
	return len(v) >= min && len(v) <= max && strings.TrimSpace(v) == v && utf8.ValidString(v) && strings.IndexFunc(v, unicode.IsControl) < 0
}
func validOptionalProfileText(v string, max int) bool {
	return len(v) <= max && strings.TrimSpace(v) == v && utf8.ValidString(v) && strings.IndexFunc(v, unicode.IsControl) < 0
}
func profileDigest(label string) eer.Digest {
	sum := sha256.Sum256([]byte("outbound.wecom.contact-profile.v1\x00" + label))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
func executedProfileResult(label string) eer.AdapterResult {
	return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: profileDigest(label), BusinessCallDispatched: true, RealExternalCallExecuted: true}
}
func unknownProfileResult(label string) eer.AdapterResult {
	return eer.AdapterResult{Completion: eer.CompletionOutcomeUnknown, ReceiptDigest: profileDigest(label), BusinessCallDispatched: true, RealExternalCallExecuted: true}
}
func finalProfileResult(label string, dispatched, real bool) eer.AdapterResult {
	return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: profileDigest(label), BusinessCallDispatched: dispatched, RealExternalCallExecuted: real}
}

var _ wecomport.ContactProfileWriter = (*WeComContactProfileClient)(nil)
