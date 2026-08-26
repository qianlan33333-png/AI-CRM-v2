package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

const maxWeComGroupMessageResponseBytes = 1 << 20

var (
	ErrInvalidWeComGroupMessage = errors.New("invalid WeCom group message")
	errWeComGroupNotDispatched  = errors.New("WeCom group message not dispatched")
	errWeComGroupOutcomeUnknown = errors.New("WeCom group message outcome unknown")
	errWeComGroupUpstream       = errors.New("WeCom group message API rejected request")
)

// GroupMessageTokenProvider keeps the outbound write boundary independent of
// the concrete credential client assembled by the composition root.
type GroupMessageTokenProvider interface {
	Token(context.Context) (string, error)
	RefreshToken(context.Context) (string, error)
}

type WeComGroupMessageClientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      GroupMessageTokenProvider
}

// WeComGroupMessageClient implements only the documented enterprise group
// message task endpoints. It does not send a message directly.
type WeComGroupMessageClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      GroupMessageTokenProvider
}

func NewWeComGroupMessageClient(config WeComGroupMessageClientConfig) (*WeComGroupMessageClient, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") || config.HTTPClient == nil || config.Token == nil {
		return nil, ErrInvalidWeComGroupMessage
	}
	return &WeComGroupMessageClient{baseURL: baseURL, httpClient: config.HTTPClient, token: config.Token}, nil
}

type GroupMessageCreateRequest struct {
	Sender      string
	ChatIDs     []string
	Text        string
	Attachments []mediaport.GroupOpsProviderReadyAttachment
}

type GroupMessageCreateResult struct {
	MessageID string
	Partial   bool
}

func (client *WeComGroupMessageClient) CreateGroupMessageTask(ctx context.Context, request GroupMessageCreateRequest) (GroupMessageCreateResult, error) {
	if client == nil || ctx == nil || !validGroupMessageText(request.Sender, 128) || !validGroupMessageBody(request.Text, request.Attachments) || len(request.ChatIDs) == 0 || len(request.ChatIDs) > 2000 {
		return GroupMessageCreateResult{}, ErrInvalidWeComGroupMessage
	}
	chatIDs := append([]string(nil), request.ChatIDs...)
	for _, chatID := range chatIDs {
		if !validGroupMessageText(chatID, 1024) {
			return GroupMessageCreateResult{}, ErrInvalidWeComGroupMessage
		}
	}
	var response struct {
		MessageID string   `json:"msgid"`
		FailList  []string `json:"fail_list"`
	}
	payload := map[string]any{
		"chat_type": "group", "chat_id_list": chatIDs, "sender": request.Sender, "allow_select": false,
	}
	if request.Text != "" {
		payload["text"] = map[string]string{"content": request.Text}
	}
	if len(request.Attachments) != 0 {
		payload["attachments"] = weComGroupMessageAttachments(request.Attachments)
	}
	if err := client.post(ctx, "/cgi-bin/externalcontact/add_msg_template", payload, &response); err != nil {
		return GroupMessageCreateResult{}, err
	}
	if !validGroupMessageText(response.MessageID, 1024) {
		return GroupMessageCreateResult{}, errWeComGroupOutcomeUnknown
	}
	return GroupMessageCreateResult{MessageID: response.MessageID, Partial: len(response.FailList) != 0}, nil
}

func validGroupMessageBody(text string, attachments []mediaport.GroupOpsProviderReadyAttachment) bool {
	if !validOptionalGroupMessageText(text, 4000) || (text == "" && len(attachments) == 0) {
		return false
	}
	return mediaport.ValidateGroupOpsProviderReadyAttachments(attachments) == nil
}

func weComGroupMessageAttachments(items []mediaport.GroupOpsProviderReadyAttachment) []map[string]any {
	result := make([]map[string]any, len(items))
	for index, item := range items {
		switch item.MsgType {
		case "image":
			result[index] = map[string]any{"msgtype": "image", "image": map[string]string{"media_id": item.MediaID}}
		case "file":
			result[index] = map[string]any{"msgtype": "file", "file": map[string]string{"media_id": item.MediaID}}
		case "miniprogram":
			result[index] = map[string]any{"msgtype": "miniprogram", "miniprogram": map[string]string{"appid": item.AppID, "page": item.PagePath, "title": item.Title, "pic_media_id": item.MediaID}}
		case "link":
			link := map[string]string{"title": item.Title, "url": item.URL}
			if item.Description != "" {
				link["desc"] = item.Description
			}
			if item.PicURL != "" {
				link["picurl"] = item.PicURL
			}
			result[index] = map[string]any{"msgtype": "link", "link": link}
		}
	}
	return result
}

func (client *WeComGroupMessageClient) post(ctx context.Context, path string, payload any, target any) error {
	if client == nil || client.httpClient == nil || client.token == nil {
		return ErrInvalidWeComGroupMessage
	}
	token, err := client.token.Token(ctx)
	if err != nil || token == "" {
		return errors.Join(errWeComGroupNotDispatched, err)
	}
	err = client.postWithToken(ctx, path, token, payload, target)
	var apiErr *weComGroupMessageAPIError
	if !errors.As(err, &apiErr) || !expiredTokenCode(apiErr.Code) {
		return err
	}
	token, refreshErr := client.token.RefreshToken(ctx)
	if refreshErr != nil || token == "" {
		return err
	}
	return client.postWithToken(ctx, path, token, payload, target)
}

func (client *WeComGroupMessageClient) postWithToken(ctx context.Context, path, token string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return ErrInvalidWeComGroupMessage
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: path})
	query := url.Values{}
	query.Set("access_token", token)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return errWeComGroupNotDispatched
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return errWeComGroupOutcomeUnknown
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errWeComGroupOutcomeUnknown
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWeComGroupMessageResponseBytes+1))
	if err != nil || len(data) > maxWeComGroupMessageResponseBytes {
		return errWeComGroupOutcomeUnknown
	}
	var base struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&base) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errWeComGroupOutcomeUnknown
	}
	if base.ErrCode != 0 {
		return fmt.Errorf("%w: %w", errWeComGroupUpstream, &weComGroupMessageAPIError{Code: base.ErrCode})
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errWeComGroupOutcomeUnknown
	}
	return nil
}

type weComGroupMessageAPIError struct{ Code int }

func (err *weComGroupMessageAPIError) Error() string {
	return fmt.Sprintf("WeCom group message API error %d", err.Code)
}

func expiredTokenCode(code int) bool { return code == 40014 || code == 42001 }

func validGroupMessageText(value string, limit int) bool {
	return value != "" && validOptionalGroupMessageText(value, limit)
}

func validOptionalGroupMessageText(value string, limit int) bool {
	return utf8.ValidString(value) && len(value) <= limit && strings.TrimSpace(value) == value
}

type groupMessageTaskCreator interface {
	CreateGroupMessageTask(context.Context, GroupMessageCreateRequest) (GroupMessageCreateResult, error)
}

// WeComGroupMessageProvider creates a staff-confirmed Group Ops task. A
// successful response is provider_accepted only, never delivery_proven.
type WeComGroupMessageProvider struct {
	client   groupMessageTaskCreator
	receipts groupopsport.GroupMessageReceiptWriter
}

var _ groupopsport.DispatchProvider = (*WeComGroupMessageProvider)(nil)

func NewWeComGroupMessageProvider(client groupMessageTaskCreator, receipts ...groupopsport.GroupMessageReceiptWriter) (*WeComGroupMessageProvider, error) {
	if client == nil {
		return nil, ErrInvalidWeComGroupMessage
	}
	if len(receipts) > 1 {
		return nil, ErrInvalidWeComGroupMessage
	}
	provider := &WeComGroupMessageProvider{client: client}
	if len(receipts) == 1 {
		if receipts[0] == nil {
			return nil, ErrInvalidWeComGroupMessage
		}
		provider.receipts = receipts[0]
	}
	return provider, nil
}

func (provider *WeComGroupMessageProvider) Dispatch(ctx context.Context, request groupopsport.DispatchRequest) (groupopsport.DispatchProviderResult, error) {
	if provider == nil || provider.client == nil || ctx == nil || !validGroupMessageDispatchRequest(request) {
		return preDispatchGroupMessageResult(request), nil
	}
	content, material, valid := groupMessageSnapshots(request)
	if !valid {
		return preDispatchGroupMessageResult(request), nil
	}
	chatID := request.TargetReference
	if !validGroupMessageText(request.SenderUserID, 128) {
		return preDispatchGroupMessageResult(request), nil
	}
	created, err := provider.client.CreateGroupMessageTask(ctx, GroupMessageCreateRequest{Sender: request.SenderUserID, ChatIDs: []string{chatID}, Text: content.MessageText, Attachments: append([]mediaport.GroupOpsProviderReadyAttachment(nil), material.Attachments...)})
	if err != nil {
		return classifyGroupMessageCreateError(err, request, request.SenderUserID, chatID), nil
	}
	label := "task"
	if created.Partial {
		label = "partial"
	}
	digest := groupMessageReceiptDigest(label, created.MessageID, request.SenderUserID, chatID)
	// The accepted task is durable before it becomes an EER success. If that
	// owner receipt cannot be stored, the real Provider call remains unknown
	// and is never replayed as a fresh create-task request.
	if provider.receipts == nil || provider.receipts.RecordGroupMessageTask(ctx, groupopsport.GroupMessageReceipt{ExecutionID: request.ExecutionID, ExternalEffectID: request.ExternalEffectID, MessageID: created.MessageID, SenderUserID: request.SenderUserID, ChatID: chatID, UserID: request.SenderUserID, TaskEvidenceDigest: digest}) != nil {
		return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchOutcomeUnknown, ReceiptDigest: groupMessageReceiptDigest("receipt-store-unknown", request.ExternalEffectID, created.MessageID), BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
	}
	if created.Partial {
		return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderRejected, ReceiptDigest: digest, BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
	}
	return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: digest, BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
}

type groupMessageSnapshot struct {
	SchemaVersion int    `json:"schema_version"`
	NodeKind      string `json:"node_kind"`
	MessageText   string `json:"message_text"`
}

type groupMessageMaterialSnapshot = mediaport.GroupOpsMaterialSnapshot

type legacyGroupMessageMaterialSnapshot struct {
	SchemaVersion int    `json:"schema_version"`
	NodeKind      string `json:"node_kind"`
	Reference     string `json:"reference"`
}

func groupMessageSnapshots(request groupopsport.DispatchRequest) (groupMessageSnapshot, groupMessageMaterialSnapshot, bool) {
	var content groupMessageSnapshot
	var material groupMessageMaterialSnapshot
	if !strictDecode(request.ContentSnapshot, &content) || content.SchemaVersion != 1 || content.NodeKind != "message" || !validOptionalGroupMessageText(content.MessageText, 4000) {
		return groupMessageSnapshot{}, groupMessageMaterialSnapshot{}, false
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if json.Unmarshal(request.MaterialSnapshot, &header) != nil {
		return groupMessageSnapshot{}, groupMessageMaterialSnapshot{}, false
	}
	if header.SchemaVersion == 1 {
		var legacy legacyGroupMessageMaterialSnapshot
		if !strictDecode(request.MaterialSnapshot, &legacy) || legacy.NodeKind != "message" || legacy.Reference != "" || !validGroupMessageText(content.MessageText, 4000) {
			return groupMessageSnapshot{}, groupMessageMaterialSnapshot{}, false
		}
		return content, material, true
	}
	if header.SchemaVersion != 2 || !strictDecode(request.MaterialSnapshot, &material) || mediaport.ValidateGroupOpsMaterialSnapshot(material) != nil || (content.MessageText == "" && len(material.Attachments) == 0) {
		return groupMessageSnapshot{}, groupMessageMaterialSnapshot{}, false
	}
	return content, material, true
}

func validGroupMessageDispatchRequest(request groupopsport.DispatchRequest) bool {
	return request.ExecutionID > 0 && strings.TrimSpace(request.ExternalEffectID) != "" && validGroupMessageText(request.TargetReference, 1024) && validGroupMessageText(request.SenderUserID, 128) &&
		validDigestValue(request.ContentDigest) && validDigestValue(request.MaterialDigest)
}

func strictDecode(raw json.RawMessage, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}

func classifyGroupMessageCreateError(err error, request groupopsport.DispatchRequest, sender, chatID string) groupopsport.DispatchProviderResult {
	switch {
	case errors.Is(err, ErrInvalidWeComGroupMessage), errors.Is(err, errWeComGroupNotDispatched):
		return preDispatchGroupMessageResult(request)
	case errors.Is(err, errWeComGroupOutcomeUnknown):
		return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchOutcomeUnknown, ReceiptDigest: groupMessageReceiptDigest("unknown", request.ExternalEffectID, sender, chatID), BusinessCallDispatched: true, RealExternalCallExecuted: true}
	case errors.Is(err, errWeComGroupUpstream):
		return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderRejected, ReceiptDigest: groupMessageReceiptDigest("rejected", request.ExternalEffectID, sender, chatID), BusinessCallDispatched: true, RealExternalCallExecuted: true}
	default:
		return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchOutcomeUnknown, ReceiptDigest: groupMessageReceiptDigest("unknown", request.ExternalEffectID, sender, chatID), BusinessCallDispatched: true, RealExternalCallExecuted: true}
	}
}

func preDispatchGroupMessageResult(request groupopsport.DispatchRequest) groupopsport.DispatchProviderResult {
	return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchPreDispatchFailure, ReceiptDigest: groupMessageReceiptDigest("pre-dispatch", request.ExternalEffectID)}
}

func groupMessageReceiptDigest(label string, values ...string) string {
	sum := sha256.Sum256([]byte("group-ops/wecom-group-message/v1\x00" + label + "\x00" + strings.Join(values, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validDigestValue(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
