package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
)

var ErrRuleNotFound = errors.New("automation rule not found")

type RuleRepository struct{}

func NewRuleRepository() *RuleRepository { return &RuleRepository{} }

func (repository *RuleRepository) ReserveRuleOperation(ctx context.Context, operation, actorScope string, key, payload [32]byte) (automationport.RuleOperationReceipt, error) {
	row, err := ruleReceiptSnapshot(ctx, operation, actorScope, key, payload)
	if err != nil {
		return automationport.RuleOperationReceipt{}, err
	}
	var digest [32]byte
	if row.ID < 1 || len(row.PayloadDigest) != len(digest) {
		return automationport.RuleOperationReceipt{}, ErrInvalidTagTrigger
	}
	copy(digest[:], row.PayloadDigest)
	return automationport.RuleOperationReceipt{ID: row.ID, PayloadDigest: digest, Result: row.ResultSnapshot}, nil
}

func (repository *RuleRepository) CompleteRuleOperation(ctx context.Context, id int64, result json.RawMessage, now time.Time) error {
	row, err := completeRuleReceipt(ctx, id, result, now)
	if err != nil || row.ID != id || len(row.ResultSnapshot) == 0 {
		return errors.Join(ErrInvalidTagTrigger, err)
	}
	return nil
}

func (repository *RuleRepository) CreateRule(ctx context.Context, rule automationport.Rule, actor int64, now time.Time) (automationport.Rule, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return automationport.Rule{}, err
	}
	condition, action, err := marshalRule(rule)
	if err != nil {
		return automationport.Rule{}, err
	}
	row, err := queries.CreateAutomationRule(ctx, automationdb.CreateAutomationRuleParams{
		AutomationCode: rule.Code, AutomationName: rule.Name, Status: string(rule.Status), TriggerType: "customer.tag_applied",
		ConditionJson: condition, ActionJson: action, ActorID: actor, Now: stamp(now),
	})
	if err != nil {
		return automationport.Rule{}, err
	}
	if err = queries.CreateAutomationRuleVersion(ctx, automationdb.CreateAutomationRuleVersionParams{
		AutomationID: row.ID, Version: row.CurrentVersion, TriggerType: row.TriggerType, ConditionJson: row.ConditionJson,
		ActionJson: row.ActionJson, Now: stamp(now), ActorID: actor,
	}); err != nil {
		return automationport.Rule{}, err
	}
	return mapRule(row)
}

func (repository *RuleRepository) UpdateRule(ctx context.Context, rule automationport.Rule, actor int64, now time.Time) (automationport.Rule, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return automationport.Rule{}, err
	}
	condition, action, err := marshalRule(rule)
	if err != nil {
		return automationport.Rule{}, err
	}
	row, err := queries.UpdateAutomationRule(ctx, automationdb.UpdateAutomationRuleParams{
		ID: int64(rule.ID), AutomationName: rule.Name, Status: string(rule.Status), ConditionJson: condition, ActionJson: action, ActorID: actor, Now: stamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return automationport.Rule{}, ErrRuleNotFound
	}
	if err != nil {
		return automationport.Rule{}, err
	}
	if err = queries.CreateAutomationRuleVersion(ctx, automationdb.CreateAutomationRuleVersionParams{
		AutomationID: row.ID, Version: row.CurrentVersion, TriggerType: row.TriggerType, ConditionJson: row.ConditionJson,
		ActionJson: row.ActionJson, Now: stamp(now), ActorID: actor,
	}); err != nil {
		return automationport.Rule{}, err
	}
	return mapRule(row)
}

func (repository *RuleRepository) SetRuleStatus(ctx context.Context, id automationport.RuleID, status automationport.RuleStatus, actor int64, now time.Time) (automationport.Rule, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return automationport.Rule{}, err
	}
	row, err := queries.SetAutomationRuleStatus(ctx, automationdb.SetAutomationRuleStatusParams{ID: int64(id), Status: string(status), ActorID: actor, Now: stamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return automationport.Rule{}, ErrRuleNotFound
	}
	if err != nil {
		return automationport.Rule{}, err
	}
	return mapRule(row)
}

func (repository *RuleRepository) GetRule(ctx context.Context, id automationport.RuleID) (automationport.Rule, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return automationport.Rule{}, err
	}
	row, err := queries.GetAutomationRule(ctx, int64(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return automationport.Rule{}, ErrRuleNotFound
	}
	if err != nil {
		return automationport.Rule{}, err
	}
	return mapRule(row)
}

func (repository *RuleRepository) ListRules(ctx context.Context) ([]automationport.Rule, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAutomationRules(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]automationport.Rule, 0, len(rows))
	for _, row := range rows {
		item, err := mapRule(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (repository *RuleRepository) ListRuleExecutions(ctx context.Context, offset, size int32) ([]automationport.RuntimeExecution, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAutomationExecutionActions(ctx, automationdb.ListAutomationExecutionActionsParams{PageOffset: offset, PageSize: size})
	if err != nil {
		return nil, err
	}
	items := make([]automationport.RuntimeExecution, 0, len(rows))
	for _, row := range rows {
		if row.ID < 1 || row.EnrollmentID < 1 || row.AutomationID < 1 || row.AutomationVersion < 1 || row.SourceEventID < 1 || row.CustomerID < 1 || !row.CreatedAt.Valid || !row.EnrolledAt.Valid {
			return nil, ErrInvalidTagTrigger
		}
		item := automationport.RuntimeExecution{ActionID: row.ID, EnrollmentID: row.EnrollmentID, AutomationID: automationport.RuleID(row.AutomationID), Version: row.AutomationVersion, SourceEventID: row.SourceEventID, CustomerID: row.CustomerID, ActionType: row.ActionType, State: row.State, CreatedAt: row.CreatedAt.Time.UTC()}
		if row.ExternalEffectID.Valid {
			value := row.ExternalEffectID.String
			item.ExternalEffectID = &value
		}
		if row.CompletedAt.Valid {
			value := row.CompletedAt.Time.UTC()
			item.CompletedAt = &value
		}
		items = append(items, item)
	}
	return items, nil
}

func marshalRule(rule automationport.Rule) ([]byte, []byte, error) {
	condition, err := json.Marshal(rule.Condition)
	if err != nil {
		return nil, nil, err
	}
	action, err := json.Marshal(rule.Action)
	if err != nil {
		return nil, nil, err
	}
	return condition, action, nil
}

func mapRule(row automationdb.Automation) (automationport.Rule, error) {
	if row.ID < 1 || row.CurrentVersion < 1 || row.TriggerType != "customer.tag_applied" || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return automationport.Rule{}, ErrInvalidTagTrigger
	}
	var condition automationport.TagAppliedCondition
	var action automationport.Action
	if json.Unmarshal(row.ConditionJson, &condition) != nil || json.Unmarshal(row.ActionJson, &action) != nil {
		return automationport.Rule{}, ErrInvalidTagTrigger
	}
	return automationport.Rule{ID: automationport.RuleID(row.ID), Code: row.AutomationCode, Name: row.AutomationName, Status: automationport.RuleStatus(row.Status), Version: row.CurrentVersion, Condition: condition, Action: action, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

func ruleReceiptSnapshot(ctx context.Context, operation, actorScope string, key, payload [32]byte) (automationdb.AutomationOperationReceipt, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return automationdb.AutomationOperationReceipt{}, err
	}
	return queries.ReserveAutomationRuleOperation(ctx, automationdb.ReserveAutomationRuleOperationParams{Operation: operation, ActorScope: actorScope, KeyDigest: key[:], PayloadDigest: payload[:]})
}

func completeRuleReceipt(ctx context.Context, id int64, snapshot []byte, now time.Time) (automationdb.AutomationOperationReceipt, error) {
	queries, err := automationQueries(ctx)
	if err != nil {
		return automationdb.AutomationOperationReceipt{}, err
	}
	return queries.CompleteAutomationRuleOperation(ctx, automationdb.CompleteAutomationRuleOperationParams{ID: id, ResultSnapshot: snapshot, Now: pgtype.Timestamptz{Time: now.UTC(), Valid: true}})
}
