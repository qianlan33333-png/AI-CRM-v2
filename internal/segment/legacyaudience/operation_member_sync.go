package legacyaudience

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const operationMembersSyncOperation ReceiptOperation = "operation_members_sync"

// SyncOperationMembers reads a complete Provider snapshot before opening the
// local UoW. Only after that read succeeds do projection replacement, receipt
// completion, and the redacted event commit together.
func (service *LocalConfigurationService) SyncOperationMembers(ctx context.Context, input OperationMemberSyncInput) (OperationMemberListResponse, error) {
	if !service.ready(ctx) || nilInterface(service.operationMemberSource) {
		return OperationMemberListResponse{}, ErrUnavailable
	}
	if input.PageSize < 1 || input.PageSize > MaximumOperationMemberPageSize || !validLocalConfigurationWrite(input.Actor, input.IdempotencyKey) {
		return OperationMemberListResponse{}, ErrInvalidInput
	}
	providerItems, err := service.operationMemberSource.ReadOperationMembers(ctx)
	if err != nil {
		return OperationMemberListResponse{}, errors.Join(ErrUnavailable, err)
	}
	if err := validateOperationMembers(providerItems); err != nil {
		return OperationMemberListResponse{}, err
	}
	sortOperationMembers(providerItems)
	payloadDigest, err := digestJSON(struct {
		Scope string            `json:"scope"`
		Items []OperationMember `json:"items"`
	}{Scope: OperationMemberScope, Items: providerItems})
	if err != nil {
		return OperationMemberListResponse{}, ErrUnavailable
	}
	raw, err := service.execute(ctx, operationMembersSyncOperation, input.Actor, input.IdempotencyKey, payloadDigest,
		func(tx context.Context, nowTime time.Time) (any, *LocalEvent, error) {
			stored, replaceErr := service.repo.ReplaceOperationMembers(tx, providerItems, nowTime)
			if replaceErr != nil {
				return nil, nil, replaceErr
			}
			if validateErr := validateOperationMembers(stored); validateErr != nil {
				return nil, nil, validateErr
			}
			sortOperationMembers(stored)
			if !sameOperationMembers(stored, providerItems) {
				return nil, nil, ErrUnavailable
			}
			response := operationMemberResponse(stored, input.PageSize)
			event, eventErr := operationMembersSyncedEvent(stored, input.Actor, input.IdempotencyKey, nowTime)
			return response, event, eventErr
		})
	if err != nil {
		return OperationMemberListResponse{}, err
	}
	return decodeMutation[OperationMemberListResponse](raw)
}

func operationMemberResponse(items []OperationMember, pageSize int) OperationMemberListResponse {
	page := make([]OperationMember, len(items))
	copy(page, items)
	if len(page) > pageSize {
		page = page[:pageSize]
	}
	return OperationMemberListResponse{
		Scope: OperationMemberScope, Items: page, PageSize: pageSize,
		ProviderReadExecuted: true, Projection: localProjection(),
	}
}

func sameOperationMembers(left, right []OperationMember) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func operationMembersSyncedEvent(items []OperationMember, actor Actor, key string, nowTime time.Time) (*LocalEvent, error) {
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	membersDigest := sha256.Sum256(raw)
	payload, err := json.Marshal(struct {
		Scope         string `json:"scope"`
		MemberCount   int    `json:"member_count"`
		MembersDigest string `json:"members_digest"`
		ActorID       int64  `json:"actor_id"`
	}{OperationMemberScope, len(items), "sha256:" + hex.EncodeToString(membersDigest[:]), actor.AdminUserID})
	if err != nil {
		return nil, err
	}
	keyDigest := sha256.Sum256([]byte(fmt.Sprintf("ai_audience.operation_members.synced\x00%d\x00%s", actor.AdminUserID, key)))
	return &LocalEvent{
		Type: "ai_audience.operation_members.synced", Payload: payload, OccurredAt: nowTime,
		IdempotencyKey: "ai-audience:" + hex.EncodeToString(keyDigest[:]),
	}, nil
}
