// Package store owns Automation configuration persistence and never directly
// writes Events-owned tables.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type AgentRepository struct{}

var _ automationport.ImageReferenceReader = (*AgentRepository)(nil)
var _ automationport.AttachmentReferenceReader = (*AgentRepository)(nil)

func NewAgentRepository() *AgentRepository { return &AgentRepository{} }

func agentQueries(ctx context.Context) (*automationdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return automationdb.New(tx), nil
}
func (r *AgentRepository) List(ctx context.Context, kind automationport.AutomationType) ([]automationport.Agent, error) {
	q, err := agentQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAutomationAgents(ctx, string(kind))
	if err != nil {
		return nil, err
	}
	result := make([]automationport.Agent, 0, len(rows))
	for _, row := range rows {
		item, e := mapAgent(row)
		if e != nil {
			return nil, e
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *AgentRepository) ListImageReferenceAgentIDs(ctx context.Context, imageID int64) ([]int64, error) {
	q, err := agentQueries(ctx)
	if r == nil || imageID < 1 {
		return nil, errors.New("automation image reference reader unavailable")
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAutomationAgentImageReferencePackages(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(rows))
	var previousID int64
	for _, row := range rows {
		if row.ID < 1 || row.ID <= previousID {
			return nil, errors.New("automation image reference reader unavailable")
		}
		previousID = row.ID
		ids, parseErr := automationImageReferenceIDs(json.RawMessage(row.ImageLibraryIds))
		if parseErr != nil {
			return nil, parseErr
		}
		for _, candidate := range ids {
			if candidate == imageID {
				result = append(result, row.ID)
				break
			}
		}
	}
	return result, nil
}

func (r *AgentRepository) ListAttachmentReferenceAgentIDs(ctx context.Context, attachmentID int64) ([]int64, error) {
	q, err := agentQueries(ctx)
	if r == nil || attachmentID < 1 {
		return nil, errors.New("automation attachment reference reader unavailable")
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAutomationAgentAttachmentReferencePackages(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(rows))
	var previousID int64
	for _, row := range rows {
		if row.ID < 1 || row.ID <= previousID {
			return nil, errors.New("automation attachment reference reader unavailable")
		}
		previousID = row.ID
		ids, parseErr := automationAttachmentReferenceIDs(json.RawMessage(row.AttachmentLibraryIds))
		if parseErr != nil {
			return nil, parseErr
		}
		for _, candidate := range ids {
			if candidate == attachmentID {
				result = append(result, row.ID)
				break
			}
		}
	}
	return result, nil
}

func automationImageReferenceIDs(raw json.RawMessage) ([]int64, error) {
	return automationReferenceIDs(raw)
}

func automationAttachmentReferenceIDs(raw json.RawMessage) ([]int64, error) {
	return automationReferenceIDs(raw)
}

func automationReferenceIDs(raw json.RawMessage) ([]int64, error) {
	if len(raw) == 0 {
		return []int64{}, nil
	}
	if raw[0] != '[' {
		return nil, errors.New("automation image reference reader unavailable")
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, errors.New("automation image reference reader unavailable")
	}
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		id, err := automationCanonicalReferenceID(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errors.New("automation image reference reader unavailable")
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func automationCanonicalImageReferenceID(raw json.RawMessage) (int64, error) {
	return automationCanonicalReferenceID(raw)
}

func automationCanonicalReferenceID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || raw[0] < '1' || raw[0] > '9' {
		return 0, errors.New("automation image reference reader unavailable")
	}
	for _, character := range raw[1:] {
		if character < '0' || character > '9' {
			return 0, errors.New("automation image reference reader unavailable")
		}
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("automation image reference reader unavailable")
	}
	return id, nil
}
func (r *AgentRepository) Get(ctx context.Context, id automationport.AgentID) (automationport.Agent, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return automationport.Agent{}, e
	}
	row, e := q.GetAutomationAgent(ctx, int64(id))
	if e != nil {
		return automationport.Agent{}, e
	}
	return mapAgent(row)
}
func (r *AgentRepository) Lock(ctx context.Context, id automationport.AgentID) (automationport.Agent, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return automationport.Agent{}, e
	}
	row, e := q.LockAutomationAgent(ctx, int64(id))
	if e != nil {
		return automationport.Agent{}, e
	}
	return mapAgent(row)
}
func (r *AgentRepository) Create(ctx context.Context, item automationport.Agent, now time.Time) (automationport.Agent, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return automationport.Agent{}, e
	}
	raw, e := json.Marshal(item.FixedContentPackage)
	if e != nil {
		return automationport.Agent{}, e
	}
	row, e := q.CreateAutomationAgent(ctx, automationdb.CreateAutomationAgentParams{AgentName: item.AgentName, AgentCode: item.AgentCode, AutomationType: string(item.AutomationType), Status: string(item.Status), DraftRolePrompt: item.DraftRolePrompt, DraftTaskPrompt: item.DraftTaskPrompt, PublishedRolePrompt: item.PublishedRolePrompt, PublishedTaskPrompt: item.PublishedTaskPrompt, DraftVersion: item.DraftVersion, PublishedVersion: item.PublishedVersion, FixedContentPackageJson: raw, CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy, CreatedAt: stamp(now), UpdatedAt: stamp(now)})
	if e != nil {
		return automationport.Agent{}, e
	}
	return mapAgent(row)
}
func (r *AgentRepository) Update(ctx context.Context, item automationport.Agent, now time.Time) (automationport.Agent, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return automationport.Agent{}, e
	}
	raw, e := json.Marshal(item.FixedContentPackage)
	if e != nil {
		return automationport.Agent{}, e
	}
	row, e := q.UpdateAutomationAgent(ctx, automationdb.UpdateAutomationAgentParams{AgentName: item.AgentName, AutomationType: string(item.AutomationType), Status: string(item.Status), DraftRolePrompt: item.DraftRolePrompt, DraftTaskPrompt: item.DraftTaskPrompt, PublishedRolePrompt: item.PublishedRolePrompt, PublishedTaskPrompt: item.PublishedTaskPrompt, DraftVersion: item.DraftVersion, PublishedVersion: item.PublishedVersion, FixedContentPackageJson: raw, UpdatedBy: item.UpdatedBy, UpdatedAt: stamp(now), ID: int64(item.ID)})
	if e != nil {
		return automationport.Agent{}, e
	}
	return mapAgent(row)
}
func (r *AgentRepository) NextCopyCode(ctx context.Context, code string) (string, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return "", e
	}
	prefix := escapeLike(code) + "_copy_"
	codes, e := q.ListAutomationAgentCodesByCopyPrefix(ctx, prefix+"%")
	if e != nil {
		return "", e
	}
	used := map[string]bool{}
	for _, value := range codes {
		used[value] = true
	}
	for index := 1; index <= 9999; index++ {
		candidate := fmt.Sprintf("%s_copy_%03d", code, index)
		if len(candidate) > 120 {
			return "", automationapp.ErrInvalidAgent
		}
		if !used[candidate] {
			return candidate, nil
		}
	}
	return "", automationapp.ErrAgentConflict
}
func (r *AgentRepository) Reserve(ctx context.Context, input automationapp.Reservation) (automationapp.Receipt, bool, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return automationapp.Receipt{}, false, e
	}
	row, e := q.ReserveAutomationAgentReceipt(ctx, automationdb.ReserveAutomationAgentReceiptParams{Operation: input.Operation, ActorScope: input.ActorScope, KeyDigest: input.KeyDigest[:], PayloadDigest: input.PayloadDigest[:], CreatedAt: stamp(input.CreatedAt)})
	if errors.Is(e, pgx.ErrNoRows) {
		row, e = q.GetAutomationAgentReceipt(ctx, automationdb.GetAutomationAgentReceiptParams{Operation: input.Operation, ActorScope: input.ActorScope, KeyDigest: input.KeyDigest[:]})
		if e != nil {
			return automationapp.Receipt{}, false, e
		}
		receipt, e := mapAgentReceipt(row)
		return receipt, false, e
	}
	if e != nil {
		return automationapp.Receipt{}, false, e
	}
	receipt, e := mapAgentReceipt(row)
	return receipt, true, e
}
func (r *AgentRepository) Complete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (automationapp.Receipt, error) {
	q, e := agentQueries(ctx)
	if e != nil {
		return automationapp.Receipt{}, e
	}
	row, e := q.CompleteAutomationAgentReceipt(ctx, automationdb.CompleteAutomationAgentReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if e != nil {
		return automationapp.Receipt{}, e
	}
	return mapAgentReceipt(row)
}

func mapAgent(row automationdb.AutomationAgentConfiguration) (automationport.Agent, error) {
	var content automationport.FixedContentPackage
	if !json.Valid(row.FixedContentPackageJson) || json.Unmarshal(row.FixedContentPackageJson, &content) != nil {
		return automationport.Agent{}, automationapp.ErrAgentUnavailable
	}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return automationport.Agent{}, automationapp.ErrAgentUnavailable
	}
	return automationport.Agent{ID: automationport.AgentID(row.ID), AgentName: row.AgentName, AgentCode: row.AgentCode, AutomationType: automationport.AutomationType(row.AutomationType), Status: automationport.AgentStatus(row.Status), DraftRolePrompt: row.DraftRolePrompt, DraftTaskPrompt: row.DraftTaskPrompt, PublishedRolePrompt: row.PublishedRolePrompt, PublishedTaskPrompt: row.PublishedTaskPrompt, DraftVersion: row.DraftVersion, PublishedVersion: row.PublishedVersion, FixedContentPackage: content, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}
func mapAgentReceipt(row automationdb.AutomationAgentOperationReceipt) (automationapp.Receipt, error) {
	if len(row.KeyDigest) != 32 || len(row.PayloadDigest) != 32 {
		return automationapp.Receipt{}, automationapp.ErrAgentUnavailable
	}
	var key, payload [32]byte
	copy(key[:], row.KeyDigest)
	copy(payload[:], row.PayloadDigest)
	return automationapp.Receipt{ID: row.ID, Operation: row.Operation, ActorScope: row.ActorScope, State: row.State, KeyDigest: key, PayloadDigest: payload, ResultSnapshot: row.ResultSnapshot}, nil
}
func stamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
func escapeLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}
