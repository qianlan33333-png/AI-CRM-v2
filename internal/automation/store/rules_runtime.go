package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
)

// RuleRuntime evaluates the deliberately closed A01 condition/action forms
// inside the existing Automation delivery transaction.  It never calls a
// provider: outbound_message remains a durable queued action for Outbound/EER.
type RuleRuntime struct{}

func NewRuleRuntime() *RuleRuntime { return &RuleRuntime{} }

type tagRuleCondition struct {
	TagID int64 `json:"tag_id"`
}

type ruleAction struct {
	Type string `json:"type"`
}

func (runtime *RuleRuntime) ExecuteTagApplied(ctx context.Context, sourceEventID, customerID, tagID int64, payload json.RawMessage, now time.Time) error {
	if runtime == nil || sourceEventID < 1 || customerID < 1 || tagID < 1 || !json.Valid(payload) || now.IsZero() {
		return ErrInvalidTagTrigger
	}
	queries, err := automationQueries(ctx)
	if err != nil {
		return err
	}
	rules, err := queries.ListActiveAutomationRulesForTagApplied(ctx)
	if err != nil {
		return err
	}
	stamp := pgtype.Timestamptz{Time: now.UTC(), Valid: true}
	for _, rule := range rules {
		condition, action, valid := decodeRuntimeRule(rule)
		if !valid || condition.TagID != tagID {
			continue
		}
		enrollment, err := queries.ReserveAutomationEnrollment(ctx, automationdb.ReserveAutomationEnrollmentParams{
			AutomationID: rule.ID, AutomationVersion: rule.CurrentVersion, SourceEventID: sourceEventID,
			CustomerID: customerID, TriggerPayload: payload, Now: stamp,
		})
		if err != nil {
			return err
		}
		actionRow, err := queries.CreateAutomationExecutionAction(ctx, automationdb.CreateAutomationExecutionActionParams{
			EnrollmentID: enrollment.ID, ActionType: action.Type, ActionSnapshot: rule.ActionJson, Now: stamp,
		})
		if err != nil {
			return err
		}
		if action.Type == "record" && actionRow.State == "queued" {
			if _, err = queries.CompleteAutomationRecordAction(ctx, automationdb.CompleteAutomationRecordActionParams{
				ActionID: actionRow.ID, Now: stamp, ReceiptDigest: textValue(ruleReceiptDigest(sourceEventID, rule.ID, rule.CurrentVersion)),
			}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
	}
	return nil
}

func decodeRuntimeRule(rule automationdb.Automation) (tagRuleCondition, ruleAction, bool) {
	var condition tagRuleCondition
	var action ruleAction
	if rule.ID < 1 || rule.CurrentVersion < 1 || rule.TriggerType != "customer.tag_applied" ||
		json.Unmarshal(rule.ConditionJson, &condition) != nil || json.Unmarshal(rule.ActionJson, &action) != nil || condition.TagID < 1 {
		return tagRuleCondition{}, ruleAction{}, false
	}
	if action.Type != "record" && action.Type != "outbound_message" {
		return tagRuleCondition{}, ruleAction{}, false
	}
	return condition, action, true
}

func ruleReceiptDigest(sourceEventID, ruleID, version int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("automation.record\x00%d\x00%d\x00%d", sourceEventID, ruleID, version)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func textValue(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
