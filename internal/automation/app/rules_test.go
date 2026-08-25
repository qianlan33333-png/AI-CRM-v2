package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

type ruleTestUOW struct{}

func (ruleTestUOW) Within(ctx context.Context, apply func(context.Context) error) error {
	return apply(ctx)
}

type ruleTestStore struct {
	rule    automationport.Rule
	receipt automationport.RuleOperationReceipt
	creates int
}

func (s *ruleTestStore) CreateRule(_ context.Context, rule automationport.Rule, _ int64, now time.Time) (automationport.Rule, error) {
	s.creates++
	rule.ID = 41
	rule.Version = 1
	rule.CreatedAt = now
	rule.UpdatedAt = now
	s.rule = rule
	return rule, nil
}
func (s *ruleTestStore) UpdateRule(_ context.Context, rule automationport.Rule, _ int64, now time.Time) (automationport.Rule, error) {
	rule.Code = s.rule.Code
	rule.Version = s.rule.Version + 1
	rule.CreatedAt = s.rule.CreatedAt
	rule.UpdatedAt = now
	s.rule = rule
	return rule, nil
}
func (s *ruleTestStore) SetRuleStatus(_ context.Context, id automationport.RuleID, status automationport.RuleStatus, _ int64, now time.Time) (automationport.Rule, error) {
	s.rule.ID = id
	s.rule.Status = status
	s.rule.UpdatedAt = now
	return s.rule, nil
}
func (s *ruleTestStore) GetRule(context.Context, automationport.RuleID) (automationport.Rule, error) {
	return s.rule, nil
}
func (s *ruleTestStore) ListRules(context.Context) ([]automationport.Rule, error) {
	return []automationport.Rule{s.rule}, nil
}
func (s *ruleTestStore) ReserveRuleOperation(_ context.Context, _ string, _ string, _ [32]byte, payload [32]byte) (automationport.RuleOperationReceipt, error) {
	if s.receipt.ID == 0 {
		s.receipt.ID = 9
		s.receipt.PayloadDigest = payload
	}
	return s.receipt, nil
}
func (s *ruleTestStore) CompleteRuleOperation(_ context.Context, _ int64, result json.RawMessage, _ time.Time) error {
	s.receipt.Result = append([]byte(nil), result...)
	return nil
}

func TestRuleServiceCreateIsDurablyReplayable(t *testing.T) {
	store := &ruleTestStore{}
	service := NewRuleService(ruleTestUOW{}, store)
	service.now = func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) }
	command := automationport.CreateRuleCommand{Code: "tag_welcome", Name: "Tag welcome", Status: automationport.RuleStatusActive, Condition: automationport.TagAppliedCondition{TagID: 7}, Action: automationport.Action{Type: "record"}, Actor: 9, IdempotencyKey: "automation-rule-test-key-0001"}
	first, err := service.CreateRule(context.Background(), command)
	if err != nil || first.ID != 41 || store.creates != 1 {
		t.Fatalf("first=%+v creates=%d err=%v", first, store.creates, err)
	}
	second, err := service.CreateRule(context.Background(), command)
	if err != nil || second.ID != first.ID || store.creates != 1 {
		t.Fatalf("replay=%+v creates=%d err=%v", second, store.creates, err)
	}
	command.Name = "different"
	if _, err = service.CreateRule(context.Background(), command); err != ErrRuleConflict {
		t.Fatalf("conflict err=%v", err)
	}
}

func TestRuleServiceRejectsUnboundedDSLAndMessagePayload(t *testing.T) {
	service := NewRuleService(ruleTestUOW{}, &ruleTestStore{})
	_, err := service.CreateRule(context.Background(), automationport.CreateRuleCommand{Code: "rule", Name: "rule", Status: automationport.RuleStatusActive, Condition: automationport.TagAppliedCondition{TagID: 0}, Action: automationport.Action{Type: "anything"}, Actor: 1, IdempotencyKey: "automation-rule-test-key-0002"})
	if err != ErrInvalidRule {
		t.Fatalf("err=%v", err)
	}
}
