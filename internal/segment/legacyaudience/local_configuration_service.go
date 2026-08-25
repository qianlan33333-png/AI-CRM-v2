package legacyaudience

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var _ LocalConfigurationApplication = (*LocalConfigurationService)(nil)

type LocalConfigurationService struct {
	uow     UnitOfWork
	repo    LocalConfigurationRepository
	agents  AutomationAgentReader
	members contactport.StaffDirectoryReader
	events  EventAppender
	now     func() time.Time
}

func NewLocalConfigurationService(
	uow UnitOfWork,
	repository LocalConfigurationRepository,
	agents AutomationAgentReader,
	members contactport.StaffDirectoryReader,
	events EventAppender,
) (*LocalConfigurationService, error) {
	if nilInterface(uow) || nilInterface(repository) || nilInterface(agents) || nilInterface(members) || nilInterface(events) {
		return nil, ErrUnavailable
	}
	return &LocalConfigurationService{uow: uow, repo: repository, agents: agents, members: members, events: events, now: time.Now}, nil
}

func (service *LocalConfigurationService) ListOperationMembers(ctx context.Context, pageSize int) (OperationMemberListResponse, error) {
	if ctx == nil || service == nil || nilInterface(service.members) {
		return OperationMemberListResponse{}, ErrUnavailable
	}
	if pageSize < 1 || pageSize > MaximumOperationMemberPageSize {
		return OperationMemberListResponse{}, ErrInvalidInput
	}
	entries, err := service.members.ListEligibleStaff(ctx)
	if err != nil {
		return OperationMemberListResponse{}, errors.Join(ErrUnavailable, err)
	}
	items := make([]OperationMember, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		userid := strings.TrimSpace(entry.WeComUserID)
		if userid == "" {
			return OperationMemberListResponse{}, ErrUnavailable
		}
		if _, duplicate := seen[userid]; duplicate {
			return OperationMemberListResponse{}, ErrUnavailable
		}
		seen[userid] = struct{}{}
		items = append(items, OperationMember{SenderUserID: userid, DisplayName: strings.TrimSpace(entry.DisplayName)})
	}
	sort.Slice(items, func(left, right int) bool { return items[left].SenderUserID < items[right].SenderUserID })
	if len(items) > pageSize {
		items = items[:pageSize]
	}
	return OperationMemberListResponse{
		Scope: OperationMemberScope, Items: items, PageSize: pageSize, Projection: localProjection(),
	}, nil
}

func (service *LocalConfigurationService) GetAutomationBinding(ctx context.Context, packageID int64) (AutomationBindingResponse, error) {
	if !service.ready(ctx) || packageID < 1 {
		return AutomationBindingResponse{}, service.invalidOrUnavailable(ctx)
	}
	if _, err := service.repo.GetPackageMetadata(ctx, packageID); err != nil {
		return AutomationBindingResponse{}, classifyServiceError(err)
	}
	binding, err := service.repo.GetAutomationBinding(ctx, packageID)
	if err != nil {
		return AutomationBindingResponse{}, classifyServiceError(err)
	}
	return AutomationBindingResponse{Binding: cloneAutomationBinding(binding), Projection: localProjection()}, nil
}

func (service *LocalConfigurationService) PutAutomationBinding(ctx context.Context, input PutAutomationBindingInput) (AutomationBindingResponse, error) {
	if !service.ready(ctx) || input.PackageID < 1 || input.AutomationAgentID < 1 || input.ExpectedVersion < 0 || !validLocalConfigurationWrite(input.Actor, input.IdempotencyKey) {
		return AutomationBindingResponse{}, ErrInvalidInput
	}
	payload, err := digestJSON(struct {
		PackageID         int64 `json:"package_id"`
		AutomationAgentID int64 `json:"automation_agent_id"`
		ExpectedVersion   int64 `json:"expected_version"`
	}{PackageID: input.PackageID, AutomationAgentID: input.AutomationAgentID, ExpectedVersion: input.ExpectedVersion})
	if err != nil {
		return AutomationBindingResponse{}, ErrUnavailable
	}
	raw, err := service.execute(ctx, ReceiptOperation("automation_binding_put"), input.Actor, input.IdempotencyKey, payload,
		func(tx context.Context, now time.Time) (any, *LocalEvent, error) {
			packageModel, lockErr := service.repo.LockPackage(tx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validateErr := validateWriteModel(packageModel); validateErr != nil {
				return nil, nil, validateErr
			}
			current, readErr := service.repo.GetAutomationBinding(tx, input.PackageID)
			if readErr != nil {
				return nil, nil, readErr
			}
			if current == nil && input.ExpectedVersion != 0 || current != nil && current.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			agent, agentErr := service.agents.GetAutomationAgent(tx, input.AutomationAgentID)
			if agentErr != nil {
				return nil, nil, agentErr
			}
			if agent.ID != input.AutomationAgentID || !selectableAutomationAgentStatus(agent.Status) {
				return nil, nil, ErrConflict
			}
			if current != nil && current.AutomationAgentID == input.AutomationAgentID {
				return AutomationBindingResponse{Binding: cloneAutomationBinding(current), Projection: localProjection()}, nil, nil
			}
			binding, saveErr := service.repo.SaveAutomationBinding(tx, AutomationBinding{
				PackageID: input.PackageID, AutomationAgentID: input.AutomationAgentID,
			}, input.Actor.AdminUserID, input.ExpectedVersion, now)
			if saveErr != nil {
				return nil, nil, saveErr
			}
			if !validAutomationBinding(binding) || binding.PackageID != input.PackageID || binding.AutomationAgentID != input.AutomationAgentID {
				return nil, nil, ErrUnavailable
			}
			response := AutomationBindingResponse{Binding: cloneAutomationBinding(&binding), Projection: localProjection()}
			event, eventErr := mutationEvent("ai_audience.package.automation_binding.updated", input.PackageID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return AutomationBindingResponse{}, err
	}
	return decodeMutation[AutomationBindingResponse](raw)
}

func (service *LocalConfigurationService) DeleteAutomationBinding(ctx context.Context, input DeleteAutomationBindingInput) (AutomationBindingDeleteResponse, error) {
	if !service.ready(ctx) || input.PackageID < 1 || input.ExpectedVersion < 0 || !validLocalConfigurationWrite(input.Actor, input.IdempotencyKey) {
		return AutomationBindingDeleteResponse{}, ErrInvalidInput
	}
	payload, err := digestJSON(struct {
		PackageID       int64 `json:"package_id"`
		ExpectedVersion int64 `json:"expected_version"`
	}{PackageID: input.PackageID, ExpectedVersion: input.ExpectedVersion})
	if err != nil {
		return AutomationBindingDeleteResponse{}, ErrUnavailable
	}
	raw, err := service.execute(ctx, ReceiptOperation("automation_binding_delete"), input.Actor, input.IdempotencyKey, payload,
		func(tx context.Context, now time.Time) (any, *LocalEvent, error) {
			packageModel, lockErr := service.repo.LockPackage(tx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validateErr := validateWriteModel(packageModel); validateErr != nil {
				return nil, nil, validateErr
			}
			current, readErr := service.repo.GetAutomationBinding(tx, input.PackageID)
			if readErr != nil {
				return nil, nil, readErr
			}
			if current == nil && input.ExpectedVersion != 0 || current != nil && current.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			deleted, deleteErr := service.repo.DeleteAutomationBinding(tx, input.PackageID, input.ExpectedVersion)
			if deleteErr != nil {
				return nil, nil, deleteErr
			}
			response := AutomationBindingDeleteResponse{PackageID: input.PackageID, Deleted: deleted, Projection: localProjection()}
			if !deleted {
				return response, nil, nil
			}
			event, eventErr := mutationEvent("ai_audience.package.automation_binding.deleted", input.PackageID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return AutomationBindingDeleteResponse{}, err
	}
	return decodeMutation[AutomationBindingDeleteResponse](raw)
}

func (service *LocalConfigurationService) GetSenders(ctx context.Context, packageID int64) (PackageSendersResponse, error) {
	if !service.ready(ctx) || packageID < 1 {
		return PackageSendersResponse{}, service.invalidOrUnavailable(ctx)
	}
	if _, err := service.repo.GetPackageMetadata(ctx, packageID); err != nil {
		return PackageSendersResponse{}, classifyServiceError(err)
	}
	items, err := service.repo.ListPackageSenders(ctx, packageID)
	if err != nil {
		return PackageSendersResponse{}, classifyServiceError(err)
	}
	if validateErr := validatePackageSenders(items); validateErr != nil {
		return PackageSendersResponse{}, validateErr
	}
	if eligibleErr := service.validateCurrentSenders(ctx, items, false); eligibleErr != nil {
		return PackageSendersResponse{}, eligibleErr
	}
	return PackageSendersResponse{PackageID: packageID, Items: clonePackageSenders(items), Projection: localProjection()}, nil
}

func (service *LocalConfigurationService) ReplaceSenders(ctx context.Context, input ReplaceSendersInput) (PackageSendersResponse, error) {
	if !service.ready(ctx) || input.PackageID < 1 || !validLocalConfigurationWrite(input.Actor, input.IdempotencyKey) {
		return PackageSendersResponse{}, ErrInvalidInput
	}
	items := clonePackageSenders(input.Items)
	if err := validatePackageSenders(items); err != nil {
		return PackageSendersResponse{}, err
	}
	payload, err := digestJSON(struct {
		PackageID int64           `json:"package_id"`
		Items     []PackageSender `json:"items"`
	}{PackageID: input.PackageID, Items: items})
	if err != nil {
		return PackageSendersResponse{}, ErrUnavailable
	}
	raw, err := service.execute(ctx, ReceiptOperation("senders_put"), input.Actor, input.IdempotencyKey, payload,
		func(tx context.Context, now time.Time) (any, *LocalEvent, error) {
			packageModel, lockErr := service.repo.LockPackage(tx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validateErr := validateWriteModel(packageModel); validateErr != nil {
				return nil, nil, validateErr
			}
			if eligibleErr := service.validateCurrentSenders(tx, items, true); eligibleErr != nil {
				return nil, nil, eligibleErr
			}
			stored, changed, saveErr := service.repo.ReplacePackageSenders(tx, input.PackageID, items, input.Actor.AdminUserID, now)
			if saveErr != nil {
				return nil, nil, saveErr
			}
			if validateErr := validatePackageSenders(stored); validateErr != nil || !samePackageSenders(stored, items) {
				return nil, nil, ErrUnavailable
			}
			response := PackageSendersResponse{PackageID: input.PackageID, Items: clonePackageSenders(stored), Projection: localProjection()}
			if !changed {
				return response, nil, nil
			}
			event, eventErr := mutationEvent("ai_audience.package.senders.replaced", input.PackageID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return PackageSendersResponse{}, err
	}
	return decodeMutation[PackageSendersResponse](raw)
}

func (service *LocalConfigurationService) GetConfiguration(ctx context.Context, packageID int64) (ConfigurationResponse, error) {
	if !service.ready(ctx) || packageID < 1 {
		return ConfigurationResponse{}, service.invalidOrUnavailable(ctx)
	}
	if _, err := service.repo.GetPackageMetadata(ctx, packageID); err != nil {
		return ConfigurationResponse{}, classifyServiceError(err)
	}
	configuration, err := service.repo.GetCurrentConfiguration(ctx, packageID)
	if err != nil {
		return ConfigurationResponse{}, classifyServiceError(err)
	}
	if configuration != nil && !validConfigurationVersion(*configuration) {
		return ConfigurationResponse{}, ErrUnavailable
	}
	return ConfigurationResponse{Configuration: cloneConfigurationVersion(configuration), Projection: localProjection()}, nil
}

func (service *LocalConfigurationService) PutConfiguration(ctx context.Context, input PutConfigurationInput) (ConfigurationResponse, error) {
	if !service.ready(ctx) || input.PackageID < 1 || input.ExpectedVersion < 0 || !validLocalConfigurationWrite(input.Actor, input.IdempotencyKey) {
		return ConfigurationResponse{}, ErrInvalidInput
	}
	template, filter, err := normalizeConfiguration(input.TemplateConfig, input.FilterConfig)
	if err != nil {
		return ConfigurationResponse{}, err
	}
	payload, err := digestJSON(struct {
		PackageID       int64           `json:"package_id"`
		ExpectedVersion int64           `json:"expected_version"`
		Template        json.RawMessage `json:"template_config"`
		Filter          json.RawMessage `json:"filter_config"`
	}{input.PackageID, input.ExpectedVersion, template, filter})
	if err != nil {
		return ConfigurationResponse{}, ErrUnavailable
	}
	raw, err := service.execute(ctx, ReceiptOperation("configuration_version_put"), input.Actor, input.IdempotencyKey, payload,
		func(tx context.Context, now time.Time) (any, *LocalEvent, error) {
			packageModel, lockErr := service.repo.LockPackage(tx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validateErr := validateWriteModel(packageModel); validateErr != nil {
				return nil, nil, validateErr
			}
			current, currentErr := service.repo.GetCurrentConfiguration(tx, input.PackageID)
			if currentErr != nil {
				return nil, nil, currentErr
			}
			currentVersion := int64(0)
			if current != nil {
				if !validConfigurationVersion(*current) {
					return nil, nil, ErrUnavailable
				}
				currentVersion = current.Version
			}
			if currentVersion != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			stored, insertErr := service.repo.InsertConfigurationVersion(tx, ConfigurationVersion{
				PackageID: input.PackageID, Version: currentVersion + 1, TemplateConfig: template, FilterConfig: filter,
				CreatedBy: input.Actor.AdminUserID, CreatedAt: now,
			})
			if insertErr != nil || !validConfigurationVersion(stored) || stored.Version != currentVersion+1 {
				if insertErr != nil {
					return nil, nil, insertErr
				}
				return nil, nil, ErrUnavailable
			}
			response := ConfigurationResponse{Configuration: cloneConfigurationVersion(&stored), Projection: localProjection()}
			event, eventErr := mutationEvent("ai_audience.package.configuration.versioned", input.PackageID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return ConfigurationResponse{}, err
	}
	return decodeMutation[ConfigurationResponse](raw)
}

func (service *LocalConfigurationService) ListSendRecords(ctx context.Context, packageID int64) (SendRecordListResponse, error) {
	if !service.ready(ctx) || packageID < 1 {
		return SendRecordListResponse{}, service.invalidOrUnavailable(ctx)
	}
	if _, err := service.repo.GetPackageMetadata(ctx, packageID); err != nil {
		return SendRecordListResponse{}, classifyServiceError(err)
	}
	items, err := service.repo.ListSendRecordProjections(ctx, packageID, 100)
	if err != nil {
		return SendRecordListResponse{}, classifyServiceError(err)
	}
	for _, item := range items {
		if !validSendRecordProjection(item) {
			return SendRecordListResponse{}, ErrUnavailable
		}
	}
	return SendRecordListResponse{PackageID: packageID, Items: append([]SendRecordProjection(nil), items...), Projection: localProjection()}, nil
}

func (service *LocalConfigurationService) validateCurrentSenders(ctx context.Context, items []PackageSender, lock bool) error {
	userIDs := make([]string, 0, len(items))
	for _, item := range items {
		userIDs = append(userIDs, item.SenderUserID)
	}
	var (
		entries []string
		err     error
	)
	if lock {
		entries, err = service.repo.LockEligibleSenderUserIDs(ctx, userIDs)
	} else {
		entries, err = service.repo.ListEligibleSenderUserIDs(ctx, userIDs)
	}
	if err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	allowed := make(map[string]struct{}, len(entries))
	for _, userid := range entries {
		userid = strings.TrimSpace(userid)
		if userid == "" {
			return ErrUnavailable
		}
		if _, duplicate := allowed[userid]; duplicate {
			return ErrUnavailable
		}
		allowed[userid] = struct{}{}
	}
	for _, item := range items {
		if _, ok := allowed[item.SenderUserID]; !ok {
			return ErrConflict
		}
	}
	return nil
}

func (service *LocalConfigurationService) execute(
	ctx context.Context,
	operation ReceiptOperation,
	actor Actor,
	idempotencyKey string,
	payloadDigest [32]byte,
	apply func(context.Context, time.Time) (any, *LocalEvent, error),
) (json.RawMessage, error) {
	if !service.ready(ctx) || apply == nil {
		return nil, ErrUnavailable
	}
	now := service.now().UTC()
	if now.IsZero() {
		return nil, ErrUnavailable
	}
	reservation := ReceiptReservation{
		Operation: operation, ActorID: actor.AdminUserID, KeyDigest: sha256.Sum256([]byte(idempotencyKey)),
		PayloadDigest: payloadDigest, CreatedAt: now,
	}
	var result json.RawMessage
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := service.repo.ReserveConfigurationReceipt(tx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !validReceipt(receipt, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrIdempotencyConflict
		}
		if owned && (receipt.State != "in_progress" || len(receipt.ResultJSON) != 0) {
			return ErrUnavailable
		}
		if !owned {
			if receipt.State != "completed" || len(receipt.ResultJSON) == 0 || !json.Valid(receipt.ResultJSON) {
				return ErrUnavailable
			}
			result = append(json.RawMessage(nil), receipt.ResultJSON...)
			return nil
		}
		value, event, applyErr := apply(tx, now)
		if applyErr != nil {
			return applyErr
		}
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil || len(encoded) == 0 || !json.Valid(encoded) {
			return errors.Join(ErrUnavailable, marshalErr)
		}
		if event != nil {
			if appendErr := service.events.Append(tx, *event); appendErr != nil {
				return appendErr
			}
		}
		completed, completeErr := service.repo.CompleteConfigurationReceipt(tx, receipt.ID, encoded, now)
		if completeErr != nil {
			return completeErr
		}
		if !validReceipt(completed, reservation) || completed.State != "completed" || completed.ID != receipt.ID ||
			subtle.ConstantTimeCompare(completed.PayloadDigest[:], payloadDigest[:]) != 1 || !equalJSON(completed.ResultJSON, encoded) {
			return ErrUnavailable
		}
		result = append(json.RawMessage(nil), encoded...)
		return nil
	})
	if err != nil {
		return nil, classifyServiceError(err)
	}
	return result, nil
}

func (service *LocalConfigurationService) ready(ctx context.Context) bool {
	return ctx != nil && service != nil && !nilInterface(service.uow) && !nilInterface(service.repo) &&
		!nilInterface(service.agents) && !nilInterface(service.members) && !nilInterface(service.events) && service.now != nil
}

func (service *LocalConfigurationService) invalidOrUnavailable(ctx context.Context) error {
	if ctx == nil || service == nil || !service.ready(ctx) {
		return ErrUnavailable
	}
	return ErrInvalidInput
}

func validLocalConfigurationWrite(actor Actor, key string) bool {
	return actor.AdminUserID > 0 && validIdempotencyKey(key)
}

func validAutomationBinding(binding AutomationBinding) bool {
	return binding.PackageID > 0 && binding.AutomationAgentID > 0 && binding.Version > 0 && binding.CreatedBy > 0 && binding.UpdatedBy > 0 &&
		!binding.CreatedAt.IsZero() && !binding.UpdatedAt.IsZero() && !binding.UpdatedAt.Before(binding.CreatedAt)
}

func selectableAutomationAgentStatus(status string) bool {
	return status == "active" || status == "paused"
}

func cloneAutomationBinding(value *AutomationBinding) *AutomationBinding {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func clonePackageSenders(items []PackageSender) []PackageSender {
	result := make([]PackageSender, len(items))
	copy(result, items)
	return result
}

func validatePackageSenders(items []PackageSender) error {
	if len(items) > MaximumSenderCount {
		return ErrInvalidInput
	}
	if items == nil {
		return ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		if item.SenderUserID == "" || item.SenderUserID != strings.TrimSpace(item.SenderUserID) {
			return ErrInvalidInput
		}
		if item.SortOrder != int32(index+1) {
			return ErrInvalidInput
		}
		if _, duplicate := seen[item.SenderUserID]; duplicate {
			return ErrInvalidInput
		}
		seen[item.SenderUserID] = struct{}{}
	}
	return nil
}

func samePackageSenders(left, right []PackageSender) bool {
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

func normalizeConfiguration(template, filter json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	if len(template) == 0 || len(filter) == 0 || int64(len(template)) > MaximumRequestBodyBytes || int64(len(filter)) > MaximumRequestBodyBytes {
		return nil, nil, ErrInvalidInput
	}
	var templateObject, filterObject map[string]any
	if json.Unmarshal(template, &templateObject) != nil || json.Unmarshal(filter, &filterObject) != nil || templateObject == nil || filterObject == nil {
		return nil, nil, ErrInvalidInput
	}
	if containsSecretConfigurationKey(templateObject) || containsSecretConfigurationKey(filterObject) {
		return nil, nil, ErrInvalidInput
	}
	return append(json.RawMessage(nil), template...), append(json.RawMessage(nil), filter...), nil
}

func containsSecretConfigurationKey(value any) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, nested := range current {
			key = strings.ToLower(key)
			if strings.Contains(key, "credential") || strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "password") ||
				key == "api_key" || key == "access_key" || key == "private_key" {
				return true
			}
			if containsSecretConfigurationKey(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range current {
			if containsSecretConfigurationKey(nested) {
				return true
			}
		}
	}
	return false
}

func validConfigurationVersion(value ConfigurationVersion) bool {
	_, _, err := normalizeConfiguration(value.TemplateConfig, value.FilterConfig)
	return value.PackageID > 0 && value.Version > 0 && value.CreatedBy > 0 && !value.CreatedAt.IsZero() && err == nil
}

func cloneConfigurationVersion(value *ConfigurationVersion) *ConfigurationVersion {
	if value == nil {
		return nil
	}
	copy := *value
	copy.TemplateConfig = append(json.RawMessage(nil), value.TemplateConfig...)
	copy.FilterConfig = append(json.RawMessage(nil), value.FilterConfig...)
	return &copy
}

func validSendRecordProjection(value SendRecordProjection) bool {
	return value.RecordID != "" && (value.State == "pending" || value.State == "unknown" || value.State == "sent" || value.State == "failed") &&
		!value.OccurredAt.IsZero() && !value.ProjectedAt.IsZero() && !value.ProjectedAt.Before(value.OccurredAt)
}
