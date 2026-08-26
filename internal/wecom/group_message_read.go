// Package wecom owns read-only WeCom integrations shared by business packages.
package wecom

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

const maxGroupMessageReadResponseBytes = 1 << 20

var (
	ErrInvalidGroupMessageRead  = errors.New("invalid WeCom group message read")
	errGroupMessageReadUnknown  = errors.New("WeCom group message read outcome unknown")
	errGroupMessageReadUpstream = errors.New("WeCom group message read API rejected request")
)

// RefreshingTokenProvider is satisfied by the existing caching provider. The
// forced refresh is only used after the documented expired-token responses.
type RefreshingTokenProvider interface {
	Token(context.Context) (wecomclient.AccessToken, error)
	RefreshToken(context.Context) (wecomclient.AccessToken, error)
}

type GroupMessageReadClientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      RefreshingTokenProvider
}

type GroupMessageReadClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      RefreshingTokenProvider
}

func NewGroupMessageReadClient(config GroupMessageReadClientConfig) (*GroupMessageReadClient, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") || config.HTTPClient == nil || config.Token == nil {
		return nil, ErrInvalidGroupMessageRead
	}
	return &GroupMessageReadClient{baseURL: baseURL, httpClient: config.HTTPClient, token: config.Token}, nil
}

type GroupMessageTask struct {
	UserID string
	Status int
}

type GroupMessageTaskPage struct {
	Items      []GroupMessageTask
	NextCursor string
}

func (client *GroupMessageReadClient) GetGroupMessageTask(ctx context.Context, messageID, cursor string) (GroupMessageTaskPage, error) {
	if client == nil || ctx == nil || !validGroupMessageReadText(messageID, 1024) || !validGroupMessageReadCursor(cursor) {
		return GroupMessageTaskPage{}, ErrInvalidGroupMessageRead
	}
	var response struct {
		NextCursor string `json:"next_cursor"`
		TaskList   []struct {
			UserID string `json:"userid"`
			Status int    `json:"status"`
		} `json:"task_list"`
	}
	if err := client.post(ctx, "/cgi-bin/externalcontact/get_groupmsg_task", groupMessageReadCursorPayload(messageID, cursor), &response); err != nil {
		return GroupMessageTaskPage{}, err
	}
	if !validGroupMessageReadCursor(response.NextCursor) {
		return GroupMessageTaskPage{}, errGroupMessageReadUnknown
	}
	items := make([]GroupMessageTask, len(response.TaskList))
	for index, item := range response.TaskList {
		if !validGroupMessageReadText(item.UserID, 128) {
			return GroupMessageTaskPage{}, errGroupMessageReadUnknown
		}
		items[index] = GroupMessageTask{UserID: item.UserID, Status: item.Status}
	}
	return GroupMessageTaskPage{Items: items, NextCursor: response.NextCursor}, nil
}

type GroupMessageSendResult struct {
	ChatID         string
	ExternalUserID string
	UserID         string
	Status         int
}

type GroupMessageSendResultPage struct {
	Items      []GroupMessageSendResult
	NextCursor string
}

func (client *GroupMessageReadClient) GetGroupMessageSendResult(ctx context.Context, messageID, userID, cursor string) (GroupMessageSendResultPage, error) {
	if client == nil || ctx == nil || !validGroupMessageReadText(messageID, 1024) || !validGroupMessageReadText(userID, 128) || !validGroupMessageReadCursor(cursor) {
		return GroupMessageSendResultPage{}, ErrInvalidGroupMessageRead
	}
	payload := groupMessageReadCursorPayload(messageID, cursor)
	payload["userid"] = userID
	var response struct {
		NextCursor string `json:"next_cursor"`
		SendList   []struct {
			ChatID         string `json:"chat_id"`
			ExternalUserID string `json:"external_userid"`
			UserID         string `json:"userid"`
			Status         int    `json:"status"`
		} `json:"send_list"`
	}
	if err := client.post(ctx, "/cgi-bin/externalcontact/get_groupmsg_send_result", payload, &response); err != nil {
		return GroupMessageSendResultPage{}, err
	}
	if !validGroupMessageReadCursor(response.NextCursor) {
		return GroupMessageSendResultPage{}, errGroupMessageReadUnknown
	}
	items := make([]GroupMessageSendResult, len(response.SendList))
	for index, item := range response.SendList {
		if !validGroupMessageReadText(item.ChatID, 1024) || !validGroupMessageReadText(item.UserID, 128) {
			return GroupMessageSendResultPage{}, errGroupMessageReadUnknown
		}
		items[index] = GroupMessageSendResult{ChatID: item.ChatID, ExternalUserID: item.ExternalUserID, UserID: item.UserID, Status: item.Status}
	}
	return GroupMessageSendResultPage{Items: items, NextCursor: response.NextCursor}, nil
}

func (client *GroupMessageReadClient) post(ctx context.Context, path string, payload any, target any) error {
	if client == nil || client.httpClient == nil || client.token == nil {
		return ErrInvalidGroupMessageRead
	}
	token, err := client.token.Token(ctx)
	if err != nil || token.Value() == "" {
		return errors.Join(errGroupMessageReadUnknown, err)
	}
	err = client.postWithToken(ctx, path, token.Value(), payload, target)
	var apiErr *groupMessageReadAPIError
	if !errors.As(err, &apiErr) || !expiredGroupMessageReadToken(apiErr.Code) {
		return err
	}
	token, refreshErr := client.token.RefreshToken(ctx)
	if refreshErr != nil || token.Value() == "" {
		return err
	}
	return client.postWithToken(ctx, path, token.Value(), payload, target)
}

func (client *GroupMessageReadClient) postWithToken(ctx context.Context, path, token string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return ErrInvalidGroupMessageRead
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: path})
	query := url.Values{}
	query.Set("access_token", token)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return errGroupMessageReadUnknown
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return errGroupMessageReadUnknown
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errGroupMessageReadUnknown
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxGroupMessageReadResponseBytes+1))
	if err != nil || len(data) > maxGroupMessageReadResponseBytes {
		return errGroupMessageReadUnknown
	}
	var base struct {
		ErrCode int `json:"errcode"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&base) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errGroupMessageReadUnknown
	}
	if base.ErrCode != 0 {
		return fmt.Errorf("%w: %w", errGroupMessageReadUpstream, &groupMessageReadAPIError{Code: base.ErrCode})
	}
	if json.Unmarshal(data, target) != nil {
		return errGroupMessageReadUnknown
	}
	return nil
}

type groupMessageReadAPIError struct{ Code int }

func (err *groupMessageReadAPIError) Error() string {
	return fmt.Sprintf("WeCom group message read API error %d", err.Code)
}
func expiredGroupMessageReadToken(code int) bool { return code == 40014 || code == 42001 }
func groupMessageReadCursorPayload(messageID, cursor string) map[string]string {
	payload := map[string]string{"msgid": messageID}
	if cursor != "" {
		payload["cursor"] = cursor
	}
	return payload
}
func validGroupMessageReadCursor(value string) bool {
	return len(value) <= 2048 && strings.TrimSpace(value) == value
}
func validGroupMessageReadText(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value
}

type groupMessageQueryClient interface {
	GetGroupMessageTask(context.Context, string, string) (GroupMessageTaskPage, error)
	GetGroupMessageSendResult(context.Context, string, string, string) (GroupMessageSendResultPage, error)
}

// GroupMessageReconciliationVerifier records delivery only when the query
// returns the exact target chat, sender userid and protocol status=1.
type GroupMessageReconciliationVerifier struct {
	client   groupMessageQueryClient
	evidence groupopsport.GroupMessageReceiptReader
}

var _ groupopsport.ReconciliationEvidenceVerifier = (*GroupMessageReconciliationVerifier)(nil)

func NewGroupMessageReconciliationVerifier(client groupMessageQueryClient, evidence groupopsport.GroupMessageReceiptReader) (*GroupMessageReconciliationVerifier, error) {
	if client == nil || evidence == nil {
		return nil, ErrInvalidGroupMessageRead
	}
	return &GroupMessageReconciliationVerifier{client: client, evidence: evidence}, nil
}

func (verifier *GroupMessageReconciliationVerifier) VerifyReconciliationEvidence(ctx context.Context, request groupopsport.ReconciliationEvidence) (groupopsport.ReconciliationEvidenceResult, error) {
	if verifier == nil || verifier.client == nil || verifier.evidence == nil || ctx == nil || request.ExecutionID < 1 || strings.TrimSpace(request.ExternalEffectID) == "" || !validGroupMessageDigest(request.EvidenceDigest) {
		return groupopsport.ReconciliationEvidenceResult{}, ErrInvalidGroupMessageRead
	}
	evidence, found, err := verifier.evidence.FindGroupMessageReceipt(ctx, request)
	if err != nil {
		return groupopsport.ReconciliationEvidenceResult{}, err
	}
	if !found || !validGroupMessageReceipt(evidence) || evidence.TaskEvidenceDigest != request.EvidenceDigest {
		return groupopsport.ReconciliationEvidenceResult{}, nil
	}
	if _, err = verifier.client.GetGroupMessageTask(ctx, evidence.MessageID, ""); err != nil {
		return groupopsport.ReconciliationEvidenceResult{}, err
	}
	seen := map[string]struct{}{}
	for cursor := ""; ; {
		if _, duplicate := seen[cursor]; duplicate {
			return groupopsport.ReconciliationEvidenceResult{}, errGroupMessageReadUnknown
		}
		seen[cursor] = struct{}{}
		page, queryErr := verifier.client.GetGroupMessageSendResult(ctx, evidence.MessageID, evidence.SenderUserID, cursor)
		if queryErr != nil {
			return groupopsport.ReconciliationEvidenceResult{}, queryErr
		}
		for _, item := range page.Items {
			if item.ChatID == evidence.ChatID && item.UserID == evidence.UserID && item.Status == 1 {
				digest := groupMessageReadDigest("delivery", evidence.MessageID, evidence.SenderUserID, evidence.ChatID, evidence.UserID, "1")
				if err = verifier.evidence.RecordGroupMessageDelivery(ctx, evidence, digest); err != nil {
					return groupopsport.ReconciliationEvidenceResult{}, err
				}
				return groupopsport.ReconciliationEvidenceResult{DeliveryProven: true, EvidenceDigest: digest}, nil
			}
		}
		if page.NextCursor == "" {
			return groupopsport.ReconciliationEvidenceResult{}, nil
		}
		cursor = page.NextCursor
	}
}

func validGroupMessageReceipt(receipt groupopsport.GroupMessageReceipt) bool {
	return receipt.ExecutionID > 0 && validGroupMessageReadText(receipt.MessageID, 1024) && validGroupMessageReadText(receipt.SenderUserID, 128) && validGroupMessageReadText(receipt.ChatID, 1024) && validGroupMessageReadText(receipt.UserID, 128) && validGroupMessageDigest(receipt.TaskEvidenceDigest)
}
func validGroupMessageDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
func groupMessageReadDigest(label string, values ...string) string {
	sum := sha256.Sum256([]byte("group-ops/wecom-group-message/v1\x00" + label + "\x00" + strings.Join(values, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
