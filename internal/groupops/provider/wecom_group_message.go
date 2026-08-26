package groupopsprovider

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

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

const maxWeComGroupMessageResponseBytes = 1 << 20

var (
	ErrInvalidWeComGroupMessage = errors.New("invalid WeCom group message")
	errWeComGroupNotDispatched  = errors.New("WeCom group message not dispatched")
	errWeComGroupOutcomeUnknown = errors.New("WeCom group message outcome unknown")
	errWeComGroupUpstream       = errors.New("WeCom group message API rejected request")
)

// GroupMessageTokenProvider is satisfied by wecomclient.CachingTokenProvider.
// Refreshing is kept at the protocol edge so stale tokens never become a
// delivery or task-creation claim.
type GroupMessageTokenProvider interface {
	Token(context.Context) (wecomclient.AccessToken, error)
	RefreshToken(context.Context) (wecomclient.AccessToken, error)
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
	Sender  string
	ChatIDs []string
	Text    string
}

type GroupMessageCreateResult struct {
	MessageID string
	Partial   bool
}

func (client *WeComGroupMessageClient) CreateGroupMessageTask(ctx context.Context, request GroupMessageCreateRequest) (GroupMessageCreateResult, error) {
	if client == nil || ctx == nil || !validGroupMessageText(request.Sender, 128) || !validGroupMessageText(request.Text, 4000) || len(request.ChatIDs) == 0 || len(request.ChatIDs) > 2000 {
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
	if err := client.post(ctx, "/cgi-bin/externalcontact/add_msg_template", map[string]any{
		"chat_type": "group", "chat_id_list": chatIDs, "sender": request.Sender,
		"allow_select": false, "text": map[string]string{"content": request.Text},
	}, &response); err != nil {
		return GroupMessageCreateResult{}, err
	}
	if !validGroupMessageText(response.MessageID, 1024) {
		return GroupMessageCreateResult{}, errWeComGroupOutcomeUnknown
	}
	return GroupMessageCreateResult{MessageID: response.MessageID, Partial: len(response.FailList) != 0}, nil
}

type GroupMessageTask struct {
	UserID string
	Status int
}

type GroupMessageTaskPage struct {
	Items      []GroupMessageTask
	NextCursor string
}

func (client *WeComGroupMessageClient) GetGroupMessageTask(ctx context.Context, messageID, cursor string) (GroupMessageTaskPage, error) {
	if client == nil || ctx == nil || !validGroupMessageText(messageID, 1024) || !validCursor(cursor) {
		return GroupMessageTaskPage{}, ErrInvalidWeComGroupMessage
	}
	var response struct {
		NextCursor string `json:"next_cursor"`
		TaskList   []struct {
			UserID string `json:"userid"`
			Status int    `json:"status"`
		} `json:"task_list"`
	}
	if err := client.post(ctx, "/cgi-bin/externalcontact/get_groupmsg_task", cursorPayload(messageID, cursor), &response); err != nil {
		return GroupMessageTaskPage{}, err
	}
	if !validCursor(response.NextCursor) {
		return GroupMessageTaskPage{}, errWeComGroupOutcomeUnknown
	}
	items := make([]GroupMessageTask, len(response.TaskList))
	for index, item := range response.TaskList {
		if !validGroupMessageText(item.UserID, 128) {
			return GroupMessageTaskPage{}, errWeComGroupOutcomeUnknown
		}
		items[index] = GroupMessageTask{UserID: item.UserID, Status: item.Status}
	}
	return GroupMessageTaskPage{Items: items, NextCursor: response.NextCursor}, nil
}

type GroupMessageSendResult struct {
	ChatID string
	UserID string
	Status int
}

type GroupMessageSendResultPage struct {
	Items      []GroupMessageSendResult
	NextCursor string
}

func (client *WeComGroupMessageClient) GetGroupMessageSendResult(ctx context.Context, messageID, userID, cursor string) (GroupMessageSendResultPage, error) {
	if client == nil || ctx == nil || !validGroupMessageText(messageID, 1024) || !validGroupMessageText(userID, 128) || !validCursor(cursor) {
		return GroupMessageSendResultPage{}, ErrInvalidWeComGroupMessage
	}
	payload := cursorPayload(messageID, cursor)
	payload["userid"] = userID
	var response struct {
		NextCursor string `json:"next_cursor"`
		SendList   []struct {
			ChatID string `json:"chat_id"`
			UserID string `json:"userid"`
			Status int    `json:"status"`
		} `json:"send_list"`
	}
	if err := client.post(ctx, "/cgi-bin/externalcontact/get_groupmsg_send_result", payload, &response); err != nil {
		return GroupMessageSendResultPage{}, err
	}
	if !validCursor(response.NextCursor) {
		return GroupMessageSendResultPage{}, errWeComGroupOutcomeUnknown
	}
	items := make([]GroupMessageSendResult, len(response.SendList))
	for index, item := range response.SendList {
		if !validGroupMessageText(item.ChatID, 1024) || !validGroupMessageText(item.UserID, 128) {
			return GroupMessageSendResultPage{}, errWeComGroupOutcomeUnknown
		}
		items[index] = GroupMessageSendResult{ChatID: item.ChatID, UserID: item.UserID, Status: item.Status}
	}
	return GroupMessageSendResultPage{Items: items, NextCursor: response.NextCursor}, nil
}

func (client *WeComGroupMessageClient) post(ctx context.Context, path string, payload any, target any) error {
	if client == nil || client.httpClient == nil || client.token == nil {
		return ErrInvalidWeComGroupMessage
	}
	token, err := client.token.Token(ctx)
	if err != nil || token.Value() == "" {
		return errors.Join(errWeComGroupNotDispatched, err)
	}
	err = client.postWithToken(ctx, path, token.Value(), payload, target)
	var apiErr *weComGroupMessageAPIError
	if !errors.As(err, &apiErr) || !expiredTokenCode(apiErr.Code) {
		return err
	}
	token, refreshErr := client.token.RefreshToken(ctx)
	if refreshErr != nil || token.Value() == "" {
		return err
	}
	return client.postWithToken(ctx, path, token.Value(), payload, target)
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

func cursorPayload(messageID, cursor string) map[string]string {
	payload := map[string]string{"msgid": messageID}
	if cursor != "" {
		payload["cursor"] = cursor
	}
	return payload
}

func validCursor(value string) bool { return len(value) <= 2048 && strings.TrimSpace(value) == value }
func validGroupMessageText(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value
}

type GroupMessageTargetResolver interface {
	ResolveGroupMessageTarget(context.Context, string) (chatID string, resolved bool, err error)
}

type groupMessageTaskCreator interface {
	CreateGroupMessageTask(context.Context, GroupMessageCreateRequest) (GroupMessageCreateResult, error)
}

// WeComGroupMessageProvider creates a staff-confirmed Group Ops task. A
// successful response is provider_accepted only, never delivery_proven.
type WeComGroupMessageProvider struct {
	client  groupMessageTaskCreator
	sender  string
	resolve GroupMessageTargetResolver
}

var _ groupopsport.DispatchProvider = (*WeComGroupMessageProvider)(nil)

func NewWeComGroupMessageProvider(client groupMessageTaskCreator, sender string, resolve GroupMessageTargetResolver) (*WeComGroupMessageProvider, error) {
	if client == nil || !validGroupMessageText(sender, 128) || resolve == nil {
		return nil, ErrInvalidWeComGroupMessage
	}
	return &WeComGroupMessageProvider{client: client, sender: sender, resolve: resolve}, nil
}

func (provider *WeComGroupMessageProvider) Dispatch(ctx context.Context, request groupopsport.DispatchRequest) (groupopsport.DispatchProviderResult, error) {
	if provider == nil || provider.client == nil || provider.resolve == nil || ctx == nil || !validGroupMessageDispatchRequest(request) {
		return preDispatchGroupMessageResult(request), nil
	}
	content, material, valid := groupMessageSnapshots(request)
	if !valid {
		return preDispatchGroupMessageResult(request), nil
	}
	chatID, resolved, err := provider.resolve.ResolveGroupMessageTarget(ctx, request.TargetReference)
	if err != nil || !resolved || !validGroupMessageText(chatID, 1024) {
		return preDispatchGroupMessageResult(request), nil
	}
	_ = material
	created, err := provider.client.CreateGroupMessageTask(ctx, GroupMessageCreateRequest{Sender: provider.sender, ChatIDs: []string{chatID}, Text: content.MessageText})
	if err != nil {
		return classifyGroupMessageCreateError(err, request, provider.sender, chatID), nil
	}
	if created.Partial {
		return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderRejected, ReceiptDigest: groupMessageReceiptDigest("partial", created.MessageID, provider.sender, chatID), BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
	}
	return groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: groupMessageReceiptDigest("task", created.MessageID, provider.sender, chatID), BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
}

type groupMessageSnapshot struct {
	SchemaVersion int    `json:"schema_version"`
	NodeKind      string `json:"node_kind"`
	MessageText   string `json:"message_text"`
}

type groupMessageMaterialSnapshot struct {
	SchemaVersion int    `json:"schema_version"`
	NodeKind      string `json:"node_kind"`
	Reference     string `json:"reference"`
}

func groupMessageSnapshots(request groupopsport.DispatchRequest) (groupMessageSnapshot, groupMessageMaterialSnapshot, bool) {
	var content groupMessageSnapshot
	var material groupMessageMaterialSnapshot
	if !strictDecode(request.ContentSnapshot, &content) || !strictDecode(request.MaterialSnapshot, &material) || content.SchemaVersion != 1 || material.SchemaVersion != 1 || content.NodeKind != "message" || material.NodeKind != "message" || !validGroupMessageText(content.MessageText, 4000) || material.Reference != "" {
		return groupMessageSnapshot{}, groupMessageMaterialSnapshot{}, false
	}
	return content, material, true
}

func validGroupMessageDispatchRequest(request groupopsport.DispatchRequest) bool {
	return request.ExecutionID > 0 && strings.TrimSpace(request.ExternalEffectID) != "" && validGroupMessageText(request.TargetReference, 1024) &&
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

type GroupMessageEvidence struct {
	MessageID string
	Sender    string
	ChatID    string
}

func (evidence GroupMessageEvidence) ReceiptDigest() string {
	return groupMessageReceiptDigest("task", evidence.MessageID, evidence.Sender, evidence.ChatID)
}

func (evidence GroupMessageEvidence) valid() bool {
	return validGroupMessageText(evidence.MessageID, 1024) && validGroupMessageText(evidence.Sender, 128) && validGroupMessageText(evidence.ChatID, 1024)
}

type GroupMessageEvidenceSource interface {
	FindGroupMessageEvidence(context.Context, groupopsport.ReconciliationEvidence) (GroupMessageEvidence, bool, error)
}

type groupMessageQueryClient interface {
	GetGroupMessageTask(context.Context, string, string) (GroupMessageTaskPage, error)
	GetGroupMessageSendResult(context.Context, string, string, string) (GroupMessageSendResultPage, error)
}

// WeComGroupMessageReconciliationVerifier verifies independently stored task
// evidence. It never trusts the ManualReconcile delivery_proven request field.
type WeComGroupMessageReconciliationVerifier struct {
	client   groupMessageQueryClient
	evidence GroupMessageEvidenceSource
}

var _ groupopsport.ReconciliationEvidenceVerifier = (*WeComGroupMessageReconciliationVerifier)(nil)

func NewWeComGroupMessageReconciliationVerifier(client groupMessageQueryClient, evidence GroupMessageEvidenceSource) (*WeComGroupMessageReconciliationVerifier, error) {
	if client == nil || evidence == nil {
		return nil, ErrInvalidWeComGroupMessage
	}
	return &WeComGroupMessageReconciliationVerifier{client: client, evidence: evidence}, nil
}

func (verifier *WeComGroupMessageReconciliationVerifier) VerifyReconciliationEvidence(ctx context.Context, request groupopsport.ReconciliationEvidence) (groupopsport.ReconciliationEvidenceResult, error) {
	if verifier == nil || verifier.client == nil || verifier.evidence == nil || ctx == nil || request.ExecutionID < 1 || strings.TrimSpace(request.ExternalEffectID) == "" || !validDigestValue(request.EvidenceDigest) {
		return groupopsport.ReconciliationEvidenceResult{}, ErrInvalidWeComGroupMessage
	}
	evidence, found, err := verifier.evidence.FindGroupMessageEvidence(ctx, request)
	if err != nil {
		return groupopsport.ReconciliationEvidenceResult{}, err
	}
	if !found || !evidence.valid() || evidence.ReceiptDigest() != request.EvidenceDigest {
		return groupopsport.ReconciliationEvidenceResult{}, nil
	}
	if _, err = verifier.client.GetGroupMessageTask(ctx, evidence.MessageID, ""); err != nil {
		return groupopsport.ReconciliationEvidenceResult{}, err
	}
	seen := map[string]struct{}{}
	for cursor := ""; ; {
		if _, duplicate := seen[cursor]; duplicate {
			return groupopsport.ReconciliationEvidenceResult{}, errWeComGroupOutcomeUnknown
		}
		seen[cursor] = struct{}{}
		page, queryErr := verifier.client.GetGroupMessageSendResult(ctx, evidence.MessageID, evidence.Sender, cursor)
		if queryErr != nil {
			return groupopsport.ReconciliationEvidenceResult{}, queryErr
		}
		for _, item := range page.Items {
			if item.ChatID == evidence.ChatID && item.UserID == evidence.Sender && item.Status == 1 {
				return groupopsport.ReconciliationEvidenceResult{DeliveryProven: true}, nil
			}
		}
		if page.NextCursor == "" {
			return groupopsport.ReconciliationEvidenceResult{}, nil
		}
		cursor = page.NextCursor
	}
}

func validDigestValue(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
