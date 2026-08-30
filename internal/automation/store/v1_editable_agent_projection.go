package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var ErrV1EditableAgentProjection = errors.New("V1 editable automation agent projection failed")

type V1EditableAgentProjection struct {
	SourceAgentID       int64
	SourceConfigID      int64
	SourcePromptID      int64
	AgentName           string
	AgentCode           string
	DraftRolePrompt     string
	DraftTaskPrompt     string
	PublishedRolePrompt string
	PublishedTaskPrompt string
	DraftVersion        int64
	PublishedVersion    int64
	LegacyConfiguration json.RawMessage
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type V1EditableAgentProjectionResult struct {
	Created  int `json:"created"`
	Replayed int `json:"replayed"`
}

func ProjectV1EditableAgents(ctx context.Context, items []V1EditableAgentProjection, actorID int64, at time.Time) (V1EditableAgentProjectionResult, error) {
	if ctx == nil || len(items) == 0 || actorID < 1 || at.IsZero() || at.Location() != time.UTC {
		return V1EditableAgentProjectionResult{}, ErrV1EditableAgentProjection
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return V1EditableAgentProjectionResult{}, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE public.automation_v1_editable_agent_projections IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return V1EditableAgentProjectionResult{}, err
	}
	result := V1EditableAgentProjectionResult{}
	for _, item := range items {
		replayed, projectErr := projectV1EditableAgent(ctx, tx, item, actorID, at)
		if projectErr != nil {
			return V1EditableAgentProjectionResult{}, projectErr
		}
		if replayed {
			result.Replayed++
		} else {
			result.Created++
		}
	}
	return result, nil
}

func projectV1EditableAgent(ctx context.Context, tx pgx.Tx, item V1EditableAgentProjection, actorID int64, at time.Time) (bool, error) {
	if item.SourceAgentID < 1 || item.SourcePromptID < 1 || item.AgentName == "" || item.AgentCode == "" || item.DraftVersion < 1 || item.PublishedVersion < 1 ||
		item.CreatedAt.IsZero() || item.UpdatedAt.Before(item.CreatedAt) || len(item.LegacyConfiguration) < 2 || item.LegacyConfiguration[0] != '{' || !json.Valid(item.LegacyConfiguration) {
		return false, ErrV1EditableAgentProjection
	}
	var agentHistoryID, promptHistoryID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM public.automation_v1_agent_history WHERE source_id=$1 AND agent_code=$2`, item.SourceAgentID, item.AgentCode).Scan(&agentHistoryID); err != nil {
		return false, err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM public.automation_v1_prompt_history WHERE source_id=$1 AND agent_code=$2`, item.SourcePromptID, item.AgentCode).Scan(&promptHistoryID); err != nil {
		return false, err
	}
	var configHistoryID *int64
	if item.SourceConfigID > 0 {
		var id int64
		if err := tx.QueryRow(ctx, `SELECT id FROM public.automation_v1_agent_config_history WHERE source_id=$1 AND agent_code=$2`, item.SourceConfigID, item.AgentCode).Scan(&id); err != nil {
			return false, err
		}
		configHistoryID = &id
	}
	var existingID int64
	err := tx.QueryRow(ctx, `
SELECT current.id
FROM public.automation_v1_editable_agent_projections AS projection
JOIN public.automation_agent_configurations AS current ON current.id=projection.agent_id
WHERE projection.agent_history_id=$1 AND projection.config_history_id IS NOT DISTINCT FROM $2 AND projection.prompt_history_id=$3 AND current.agent_code=$4`,
		agentHistoryID, configHistoryID, promptHistoryID, item.AgentCode).Scan(&existingID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var conflictingID int64
	err = tx.QueryRow(ctx, `SELECT id FROM public.automation_agent_configurations WHERE agent_code=$1`, item.AgentCode).Scan(&conflictingID)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return false, ErrV1EditableAgentProjection
	}
	emptyContent := json.RawMessage(`{"content_text":"","image_library_ids":[],"miniprogram_library_ids":[],"attachment_library_ids":[],"group_invite_library_ids":[]}`)
	var agentID int64
	err = tx.QueryRow(ctx, `
INSERT INTO public.automation_agent_configurations
  (agent_name,agent_code,automation_type,status,draft_role_prompt,draft_task_prompt,
   published_role_prompt,published_task_prompt,draft_version,published_version,
   fixed_content_package_json,legacy_configuration_json,execution_enabled,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,false,$13,$13,$14,$15)
RETURNING id`, item.AgentName, item.AgentCode, string(automationport.AutomationTypeAgent), string(automationport.AgentStatusPaused),
		item.DraftRolePrompt, item.DraftTaskPrompt, item.PublishedRolePrompt, item.PublishedTaskPrompt,
		item.DraftVersion, item.PublishedVersion, emptyContent, item.LegacyConfiguration, actorID, item.CreatedAt, item.UpdatedAt).Scan(&agentID)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO public.automation_v1_editable_agent_projections
  (agent_history_id,config_history_id,prompt_history_id,agent_id,created_at)
VALUES ($1,$2,$3,$4,$5)`, agentHistoryID, configHistoryID, promptHistoryID, agentID, at); err != nil {
		return false, err
	}
	return false, nil
}
