package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidRule     = errors.New("invalid automation rule")
	ErrRuleConflict    = errors.New("automation rule command conflict")
	ErrRuleUnavailable = errors.New("automation rule service unavailable")
)

var ruleCode = regexp.MustCompile(`^[a-z0-9_-]{1,120}$`)

type RuleStore interface {
	CreateRule(context.Context, automationport.Rule, int64, time.Time) (automationport.Rule, error)
	UpdateRule(context.Context, automationport.Rule, int64, time.Time) (automationport.Rule, error)
	SetRuleStatus(context.Context, automationport.RuleID, automationport.RuleStatus, int64, time.Time) (automationport.Rule, error)
	GetRule(context.Context, automationport.RuleID) (automationport.Rule, error)
	ListRules(context.Context) ([]automationport.Rule, error)
	ReserveRuleOperation(context.Context, string, string, [32]byte, [32]byte) (automationport.RuleOperationReceipt, error)
	CompleteRuleOperation(context.Context, int64, json.RawMessage, time.Time) error
}

type RuleService struct {
	uow   platformport.UnitOfWork
	store RuleStore
	now   func() time.Time
}

var _ automationport.RuleService = (*RuleService)(nil)

func NewRuleService(uow platformport.UnitOfWork, store RuleStore) *RuleService {
	return &RuleService{uow: uow, store: store, now: time.Now}
}

func (service *RuleService) CreateRule(ctx context.Context, command automationport.CreateRuleCommand) (automationport.Rule, error) {
	rule := automationport.Rule{Code: command.Code, Name: command.Name, Status: command.Status, Condition: command.Condition, Action: command.Action}
	return service.mutate(ctx, "create", command.Actor, command.IdempotencyKey, rule, func(tx context.Context, now time.Time) (automationport.Rule, error) {
		return service.store.CreateRule(tx, rule, command.Actor, now)
	})
}

func (service *RuleService) UpdateRule(ctx context.Context, command automationport.UpdateRuleCommand) (automationport.Rule, error) {
	rule := automationport.Rule{ID: command.ID, Name: command.Name, Status: command.Status, Condition: command.Condition, Action: command.Action}
	return service.mutate(ctx, "update", command.Actor, command.IdempotencyKey, rule, func(tx context.Context, now time.Time) (automationport.Rule, error) {
		return service.store.UpdateRule(tx, rule, command.Actor, now)
	})
}

func (service *RuleService) SetRuleStatus(ctx context.Context, id automationport.RuleID, status automationport.RuleStatus, actor int64, key string) (automationport.Rule, error) {
	rule := automationport.Rule{ID: id, Status: status}
	return service.mutate(ctx, "set_status", actor, key, rule, func(tx context.Context, now time.Time) (automationport.Rule, error) {
		return service.store.SetRuleStatus(tx, id, status, actor, now)
	})
}

func (service *RuleService) GetRule(ctx context.Context, id automationport.RuleID) (automationport.Rule, error) {
	if service == nil || service.uow == nil || service.store == nil || ctx == nil || id < 1 {
		return automationport.Rule{}, ErrInvalidRule
	}
	var result automationport.Rule
	err := service.uow.Within(ctx, func(tx context.Context) error { var err error; result, err = service.store.GetRule(tx, id); return err })
	return result, classifyRule(err)
}

func (service *RuleService) ListRules(ctx context.Context) ([]automationport.Rule, error) {
	if service == nil || service.uow == nil || service.store == nil || ctx == nil {
		return nil, ErrInvalidRule
	}
	var result []automationport.Rule
	err := service.uow.Within(ctx, func(tx context.Context) error { var err error; result, err = service.store.ListRules(tx); return err })
	return result, classifyRule(err)
}

func (service *RuleService) mutate(ctx context.Context, operation string, actor int64, key string, command automationport.Rule, apply func(context.Context, time.Time) (automationport.Rule, error)) (automationport.Rule, error) {
	if service == nil || service.uow == nil || service.store == nil || service.now == nil || ctx == nil || apply == nil || !validRuleCommand(operation, actor, key, command) {
		return automationport.Rule{}, ErrInvalidRule
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return automationport.Rule{}, ErrInvalidRule
	}
	keyDigest, payloadDigest := sha256.Sum256([]byte(key)), sha256.Sum256(payload)
	var result automationport.Rule
	err = service.uow.Within(ctx, func(tx context.Context) error {
		receipt, err := service.store.ReserveRuleOperation(tx, operation, "admin:"+itoa(actor), keyDigest, payloadDigest)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrRuleConflict
		}
		if len(receipt.Result) > 0 {
			if json.Unmarshal(receipt.Result, &result) != nil || !validStoredRule(result) {
				return ErrRuleUnavailable
			}
			return nil
		}
		result, err = apply(tx, service.now().UTC())
		if err != nil {
			return err
		}
		if !validStoredRule(result) {
			return ErrRuleUnavailable
		}
		snapshot, err := json.Marshal(result)
		if err != nil {
			return ErrRuleUnavailable
		}
		return service.store.CompleteRuleOperation(tx, receipt.ID, snapshot, service.now().UTC())
	})
	return result, classifyRule(err)
}

func validRuleCommand(operation string, actor int64, key string, rule automationport.Rule) bool {
	if actor < 1 || len(key) < 16 || len(key) > 128 || strings.TrimSpace(key) != key {
		return false
	}
	if !validRuleStatus(rule.Status) || !validRuleAction(rule.Action) {
		return false
	}
	if operation == "set_status" {
		return rule.ID > 0
	}
	return (operation == "create" && rule.ID == 0 && ruleCode.MatchString(rule.Code) || operation == "update" && rule.ID > 0) &&
		strings.TrimSpace(rule.Name) == rule.Name && len(rule.Name) > 0 && len(rule.Name) <= 120 && rule.Condition.TagID > 0
}

func validRuleStatus(status automationport.RuleStatus) bool {
	return status == automationport.RuleStatusActive || status == automationport.RuleStatusPaused || status == automationport.RuleStatusArchived
}
func validRuleAction(action automationport.Action) bool {
	switch action.Type {
	case "record":
		return action.TemplateKey == ""
	case "outbound_message":
		return action.TemplateKey == outboundapp.TemplateTextNoticeV1
	default:
		return false
	}
}
func validStoredRule(rule automationport.Rule) bool {
	return rule.ID > 0 && rule.Version > 0 && (rule.Status == automationport.RuleStatusArchived || (ruleCode.MatchString(rule.Code) && strings.TrimSpace(rule.Name) == rule.Name && rule.Name != "" && rule.Condition.TagID > 0 && validRuleAction(rule.Action)))
}
func itoa(value int64) string { return strconv.FormatInt(value, 10) }
func classifyRule(err error) error {
	if err == nil || errors.Is(err, ErrInvalidRule) || errors.Is(err, ErrRuleConflict) {
		return err
	}
	return errors.Join(ErrRuleUnavailable, err)
}
