package store

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
)

var _ automationport.AutomationHistoryStore = (*Repository)(nil)

// AutomationHistoryReader reads only V1 history tables. It does not create a
// current automation, rule, prompt, agent, publish, event, or Provider effect.
type AutomationHistoryReader struct{ db automationdb.DBTX }

var _ automationport.AutomationHistoryReader = (*AutomationHistoryReader)(nil)

func NewAutomationHistoryReader(db automationdb.DBTX) *AutomationHistoryReader {
	return &AutomationHistoryReader{db: db}
}

func (repository *Repository) CreateHistoricalAutomationSOP(ctx context.Context, value automationport.HistoricalAutomationSOP) (automationport.HistoricalAutomationSOP, error) {
	if repository == nil || value.ID != 0 || !validAutomationHistorySOPCreate(value) {
		return automationport.HistoricalAutomationSOP{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := automationQueries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationSOP{}, automationHistoryStoreError(err)
	}
	row, err := q.CreateHistoricalAutomationSOP(ctx, automationdb.CreateHistoricalAutomationSOPParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], PoolKey: value.PoolKey,
		DayIndex: value.DayIndex, ContentMasked: value.ContentMasked, ImagesDigest: value.ImagesDigest[:], OriginalEnabled: value.OriginalEnabled,
		CreatedAt: stamp(value.CreatedAt), UpdatedAt: stamp(value.UpdatedAt),
	})
	if err != nil {
		return automationport.HistoricalAutomationSOP{}, automationHistoryStoreError(err)
	}
	return automationHistorySOP(row)
}

func (repository *Repository) GetHistoricalAutomationSOP(ctx context.Context, id int64) (automationport.HistoricalAutomationSOP, error) {
	q, err := automationHistoryQueries(repository, ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationSOP{}, err
	}
	row, err := q.GetHistoricalAutomationSOP(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationSOP{}, automationHistoryStoreError(err)
	}
	return automationHistorySOP(row)
}

func (repository *Repository) CreateHistoricalAutomationConfig(ctx context.Context, value automationport.HistoricalAutomationConfig) (automationport.HistoricalAutomationConfig, error) {
	if repository == nil || value.ID != 0 || !validAutomationHistoryConfigCreate(value) {
		return automationport.HistoricalAutomationConfig{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := automationQueries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationConfig{}, automationHistoryStoreError(err)
	}
	row, err := q.CreateHistoricalAutomationConfig(ctx, automationdb.CreateHistoricalAutomationConfigParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], AgentCode: value.AgentCode,
		DisplayName: value.DisplayName, ScenarioCode: value.ScenarioCode, OriginalEnabled: value.OriginalEnabled, DraftVersion: value.DraftVersion,
		PublishedVersion: value.PublishedVersion, PublishedAt: value.PublishedAt, LastModifiedAt: value.LastModifiedAt,
		LastModifiedSource: value.LastModifiedSource, SubmittedForPublish: value.SubmittedForPublish, SubmittedAt: value.SubmittedAt,
		CreatedAt: stamp(value.CreatedAt), UpdatedAt: stamp(value.UpdatedAt), ActorsDigest: value.ActorsDigest[:], ConfigDigest: value.ConfigDigest[:],
	})
	if err != nil {
		return automationport.HistoricalAutomationConfig{}, automationHistoryStoreError(err)
	}
	return automationHistoryConfig(row)
}

func (repository *Repository) GetHistoricalAutomationConfig(ctx context.Context, id int64) (automationport.HistoricalAutomationConfig, error) {
	q, err := automationHistoryQueries(repository, ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationConfig{}, err
	}
	row, err := q.GetHistoricalAutomationConfig(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationConfig{}, automationHistoryStoreError(err)
	}
	return automationHistoryConfig(row)
}

func (repository *Repository) CreateHistoricalAutomationPrompt(ctx context.Context, value automationport.HistoricalAutomationPrompt) (automationport.HistoricalAutomationPrompt, error) {
	if repository == nil || value.ID != 0 || !validAutomationHistoryPromptCreate(value) {
		return automationport.HistoricalAutomationPrompt{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := automationQueries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationPrompt{}, automationHistoryStoreError(err)
	}
	row, err := q.CreateHistoricalAutomationPrompt(ctx, automationdb.CreateHistoricalAutomationPromptParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], AgentCode: value.AgentCode,
		DisplayName: value.DisplayName, OriginalEnabled: value.OriginalEnabled, Version: value.Version, CreatedAt: stamp(value.CreatedAt),
		UpdatedAt: stamp(value.UpdatedAt), PromptDigest: value.PromptDigest[:],
	})
	if err != nil {
		return automationport.HistoricalAutomationPrompt{}, automationHistoryStoreError(err)
	}
	return automationHistoryPrompt(row)
}

func (repository *Repository) GetHistoricalAutomationPrompt(ctx context.Context, id int64) (automationport.HistoricalAutomationPrompt, error) {
	q, err := automationHistoryQueries(repository, ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationPrompt{}, err
	}
	row, err := q.GetHistoricalAutomationPrompt(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationPrompt{}, automationHistoryStoreError(err)
	}
	return automationHistoryPrompt(row)
}

func (repository *Repository) CreateHistoricalAutomationAgent(ctx context.Context, value automationport.HistoricalAutomationAgent) (automationport.HistoricalAutomationAgent, error) {
	if repository == nil || value.ID != 0 || !validAutomationHistoryAgentCreate(value) {
		return automationport.HistoricalAutomationAgent{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := automationQueries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationAgent{}, automationHistoryStoreError(err)
	}
	row, err := q.CreateHistoricalAutomationAgent(ctx, automationdb.CreateHistoricalAutomationAgentParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], ProgramSourceID: value.ProgramSourceID,
		WorkflowSourceID: value.WorkflowSourceID, NodeSourceID: value.NodeSourceID, TaskSourceID: value.TaskSourceID, AgentCode: value.AgentCode,
		AgentName: value.AgentName, OriginalType: value.OriginalType, OriginalStatus: value.OriginalStatus, SortOrder: value.SortOrder,
		OriginalEnabled: value.OriginalEnabled, CreatedAt: stamp(value.CreatedAt), UpdatedAt: stamp(value.UpdatedAt), ArchivedAt: value.ArchivedAt,
		ActorsDigest: value.ActorsDigest[:], ConfigurationDigest: value.ConfigurationDigest[:],
	})
	if err != nil {
		return automationport.HistoricalAutomationAgent{}, automationHistoryStoreError(err)
	}
	return automationHistoryAgent(row)
}

func (repository *Repository) GetHistoricalAutomationAgent(ctx context.Context, id int64) (automationport.HistoricalAutomationAgent, error) {
	q, err := automationHistoryQueries(repository, ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationAgent{}, err
	}
	row, err := q.GetHistoricalAutomationAgent(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationAgent{}, automationHistoryStoreError(err)
	}
	return automationHistoryAgent(row)
}

func (reader *AutomationHistoryReader) GetHistoricalAutomationSOP(ctx context.Context, id int64) (automationport.HistoricalAutomationSOP, error) {
	if id < 1 {
		return automationport.HistoricalAutomationSOP{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationSOP{}, automationHistoryReaderError(err)
	}
	row, err := q.GetHistoricalAutomationSOP(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationSOP{}, automationHistoryStoreError(err)
	}
	return automationHistorySOP(row)
}

func (reader *AutomationHistoryReader) ListHistoricalAutomationSOPs(ctx context.Context, page automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationSOP, int64, error) {
	values := make([]automationport.HistoricalAutomationSOP, 0)
	if !validAutomationHistoryPage(page) {
		return values, 0, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return values, 0, automationHistoryReaderError(err)
	}
	rows, err := q.ListHistoricalAutomationSOPs(ctx, automationdb.ListHistoricalAutomationSOPsParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return values, 0, automationHistoryStoreError(err)
	}
	total, err := q.CountHistoricalAutomationSOPs(ctx)
	if err != nil || total < 0 {
		return values, 0, automationHistoryReaderError(err)
	}
	values = make([]automationport.HistoricalAutomationSOP, 0, len(rows))
	for _, row := range rows {
		value, mapErr := automationHistorySOP(row)
		if mapErr != nil {
			return make([]automationport.HistoricalAutomationSOP, 0), 0, automationport.ErrAutomationHistoryUnavailable
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (reader *AutomationHistoryReader) GetHistoricalAutomationConfig(ctx context.Context, id int64) (automationport.HistoricalAutomationConfig, error) {
	if id < 1 {
		return automationport.HistoricalAutomationConfig{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationConfig{}, automationHistoryReaderError(err)
	}
	row, err := q.GetHistoricalAutomationConfig(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationConfig{}, automationHistoryStoreError(err)
	}
	return automationHistoryConfig(row)
}

func (reader *AutomationHistoryReader) ListHistoricalAutomationConfigs(ctx context.Context, page automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationConfig, int64, error) {
	values := make([]automationport.HistoricalAutomationConfig, 0)
	if !validAutomationHistoryPage(page) {
		return values, 0, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return values, 0, automationHistoryReaderError(err)
	}
	rows, err := q.ListHistoricalAutomationConfigs(ctx, automationdb.ListHistoricalAutomationConfigsParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return values, 0, automationHistoryStoreError(err)
	}
	total, err := q.CountHistoricalAutomationConfigs(ctx)
	if err != nil || total < 0 {
		return values, 0, automationHistoryReaderError(err)
	}
	values = make([]automationport.HistoricalAutomationConfig, 0, len(rows))
	for _, row := range rows {
		value, mapErr := automationHistoryConfig(row)
		if mapErr != nil {
			return make([]automationport.HistoricalAutomationConfig, 0), 0, automationport.ErrAutomationHistoryUnavailable
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (reader *AutomationHistoryReader) GetHistoricalAutomationPrompt(ctx context.Context, id int64) (automationport.HistoricalAutomationPrompt, error) {
	if id < 1 {
		return automationport.HistoricalAutomationPrompt{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationPrompt{}, automationHistoryReaderError(err)
	}
	row, err := q.GetHistoricalAutomationPrompt(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationPrompt{}, automationHistoryStoreError(err)
	}
	return automationHistoryPrompt(row)
}

func (reader *AutomationHistoryReader) ListHistoricalAutomationPrompts(ctx context.Context, page automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationPrompt, int64, error) {
	values := make([]automationport.HistoricalAutomationPrompt, 0)
	if !validAutomationHistoryPage(page) {
		return values, 0, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return values, 0, automationHistoryReaderError(err)
	}
	rows, err := q.ListHistoricalAutomationPrompts(ctx, automationdb.ListHistoricalAutomationPromptsParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return values, 0, automationHistoryStoreError(err)
	}
	total, err := q.CountHistoricalAutomationPrompts(ctx)
	if err != nil || total < 0 {
		return values, 0, automationHistoryReaderError(err)
	}
	values = make([]automationport.HistoricalAutomationPrompt, 0, len(rows))
	for _, row := range rows {
		value, mapErr := automationHistoryPrompt(row)
		if mapErr != nil {
			return make([]automationport.HistoricalAutomationPrompt, 0), 0, automationport.ErrAutomationHistoryUnavailable
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (reader *AutomationHistoryReader) GetHistoricalAutomationAgent(ctx context.Context, id int64) (automationport.HistoricalAutomationAgent, error) {
	if id < 1 {
		return automationport.HistoricalAutomationAgent{}, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return automationport.HistoricalAutomationAgent{}, automationHistoryReaderError(err)
	}
	row, err := q.GetHistoricalAutomationAgent(ctx, id)
	if err != nil {
		return automationport.HistoricalAutomationAgent{}, automationHistoryStoreError(err)
	}
	return automationHistoryAgent(row)
}

func (reader *AutomationHistoryReader) ListHistoricalAutomationAgents(ctx context.Context, page automationport.AutomationHistoryQuery) ([]automationport.HistoricalAutomationAgent, int64, error) {
	values := make([]automationport.HistoricalAutomationAgent, 0)
	if !validAutomationHistoryPage(page) {
		return values, 0, automationport.ErrAutomationHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return values, 0, automationHistoryReaderError(err)
	}
	rows, err := q.ListHistoricalAutomationAgents(ctx, automationdb.ListHistoricalAutomationAgentsParams{RowLimit: page.Limit, RowOffset: page.Offset})
	if err != nil {
		return values, 0, automationHistoryStoreError(err)
	}
	total, err := q.CountHistoricalAutomationAgents(ctx)
	if err != nil || total < 0 {
		return values, 0, automationHistoryReaderError(err)
	}
	values = make([]automationport.HistoricalAutomationAgent, 0, len(rows))
	for _, row := range rows {
		value, mapErr := automationHistoryAgent(row)
		if mapErr != nil {
			return make([]automationport.HistoricalAutomationAgent, 0), 0, automationport.ErrAutomationHistoryUnavailable
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (reader *AutomationHistoryReader) queries(ctx context.Context) (*automationdb.Queries, error) {
	if reader == nil || nilAutomationHistoryStoreDependency(reader.db) || ctx == nil {
		return nil, automationport.ErrAutomationHistoryUnavailable
	}
	return automationdb.New(reader.db), nil
}

func automationHistoryQueries(repository *Repository, ctx context.Context, id int64) (*automationdb.Queries, error) {
	if repository == nil || id < 1 {
		return nil, automationport.ErrAutomationHistoryInvalid
	}
	q, err := automationQueries(ctx)
	if err != nil {
		return nil, automationHistoryStoreError(err)
	}
	return q, nil
}

func automationHistorySOP(row automationdb.AutomationV1SopHistory) (automationport.HistoricalAutomationSOP, error) {
	value := automationport.HistoricalAutomationSOP{HistoricalAutomationIdentity: automationHistoryIdentity(row.ID, row.SourceID, row.SourceKeyDigest, row.SourcePayloadDigest), PoolKey: row.PoolKey, DayIndex: row.DayIndex, ContentMasked: row.ContentMasked, OriginalEnabled: row.OriginalEnabled}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid || !copyAutomationHistoryDigest(&value.ImagesDigest, row.ImagesDigest) {
		return automationport.HistoricalAutomationSOP{}, automationport.ErrAutomationHistoryUnavailable
	}
	value.CreatedAt, value.UpdatedAt = row.CreatedAt.Time.UTC(), row.UpdatedAt.Time.UTC()
	if _, err := automationapp.HistoricalAutomationSOPDigest(value); err != nil {
		return automationport.HistoricalAutomationSOP{}, automationport.ErrAutomationHistoryUnavailable
	}
	return value, nil
}

func automationHistoryConfig(row automationdb.AutomationV1AgentConfigHistory) (automationport.HistoricalAutomationConfig, error) {
	value := automationport.HistoricalAutomationConfig{HistoricalAutomationIdentity: automationHistoryIdentity(row.ID, row.SourceID, row.SourceKeyDigest, row.SourcePayloadDigest), AgentCode: row.AgentCode, DisplayName: row.DisplayName, ScenarioCode: row.ScenarioCode, OriginalEnabled: row.OriginalEnabled, DraftVersion: row.DraftVersion, PublishedVersion: row.PublishedVersion, PublishedAt: row.PublishedAt, LastModifiedAt: row.LastModifiedAt, LastModifiedSource: row.LastModifiedSource, SubmittedForPublish: row.SubmittedForPublish, SubmittedAt: row.SubmittedAt}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid || !copyAutomationHistoryDigest(&value.ActorsDigest, row.ActorsDigest) || !copyAutomationHistoryDigest(&value.ConfigDigest, row.ConfigDigest) {
		return automationport.HistoricalAutomationConfig{}, automationport.ErrAutomationHistoryUnavailable
	}
	value.CreatedAt, value.UpdatedAt = row.CreatedAt.Time.UTC(), row.UpdatedAt.Time.UTC()
	if _, err := automationapp.HistoricalAutomationConfigDigest(value); err != nil {
		return automationport.HistoricalAutomationConfig{}, automationport.ErrAutomationHistoryUnavailable
	}
	return value, nil
}

func automationHistoryPrompt(row automationdb.AutomationV1PromptHistory) (automationport.HistoricalAutomationPrompt, error) {
	value := automationport.HistoricalAutomationPrompt{HistoricalAutomationIdentity: automationHistoryIdentity(row.ID, row.SourceID, row.SourceKeyDigest, row.SourcePayloadDigest), AgentCode: row.AgentCode, DisplayName: row.DisplayName, OriginalEnabled: row.OriginalEnabled, Version: row.Version}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid || !copyAutomationHistoryDigest(&value.PromptDigest, row.PromptDigest) {
		return automationport.HistoricalAutomationPrompt{}, automationport.ErrAutomationHistoryUnavailable
	}
	value.CreatedAt, value.UpdatedAt = row.CreatedAt.Time.UTC(), row.UpdatedAt.Time.UTC()
	if _, err := automationapp.HistoricalAutomationPromptDigest(value); err != nil {
		return automationport.HistoricalAutomationPrompt{}, automationport.ErrAutomationHistoryUnavailable
	}
	return value, nil
}

func automationHistoryAgent(row automationdb.AutomationV1AgentHistory) (automationport.HistoricalAutomationAgent, error) {
	value := automationport.HistoricalAutomationAgent{HistoricalAutomationIdentity: automationHistoryIdentity(row.ID, row.SourceID, row.SourceKeyDigest, row.SourcePayloadDigest), ProgramSourceID: row.ProgramSourceID, WorkflowSourceID: row.WorkflowSourceID, NodeSourceID: row.NodeSourceID, TaskSourceID: row.TaskSourceID, AgentCode: row.AgentCode, AgentName: row.AgentName, OriginalType: row.OriginalType, OriginalStatus: row.OriginalStatus, SortOrder: row.SortOrder, OriginalEnabled: row.OriginalEnabled, ArchivedAt: row.ArchivedAt}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid || !copyAutomationHistoryDigest(&value.ActorsDigest, row.ActorsDigest) || !copyAutomationHistoryDigest(&value.ConfigurationDigest, row.ConfigurationDigest) {
		return automationport.HistoricalAutomationAgent{}, automationport.ErrAutomationHistoryUnavailable
	}
	value.CreatedAt, value.UpdatedAt = row.CreatedAt.Time.UTC(), row.UpdatedAt.Time.UTC()
	if _, err := automationapp.HistoricalAutomationAgentDigest(value); err != nil {
		return automationport.HistoricalAutomationAgent{}, automationport.ErrAutomationHistoryUnavailable
	}
	return value, nil
}

func automationHistoryIdentity(id, sourceID int64, sourceKey, payload []byte) automationport.HistoricalAutomationIdentity {
	value := automationport.HistoricalAutomationIdentity{ID: id, SourceID: sourceID}
	_ = copyAutomationHistoryDigest(&value.SourceKeyDigest, sourceKey)
	_ = copyAutomationHistoryDigest(&value.SourcePayloadDigest, payload)
	return value
}

func validAutomationHistorySOPCreate(value automationport.HistoricalAutomationSOP) bool {
	value.ID = 1
	_, err := automationapp.HistoricalAutomationSOPDigest(value)
	return err == nil
}
func validAutomationHistoryConfigCreate(value automationport.HistoricalAutomationConfig) bool {
	value.ID = 1
	_, err := automationapp.HistoricalAutomationConfigDigest(value)
	return err == nil
}
func validAutomationHistoryPromptCreate(value automationport.HistoricalAutomationPrompt) bool {
	value.ID = 1
	_, err := automationapp.HistoricalAutomationPromptDigest(value)
	return err == nil
}
func validAutomationHistoryAgentCreate(value automationport.HistoricalAutomationAgent) bool {
	value.ID = 1
	_, err := automationapp.HistoricalAutomationAgentDigest(value)
	return err == nil
}

func copyAutomationHistoryDigest(target *[32]byte, raw []byte) bool {
	if target == nil || len(raw) != len(*target) {
		return false
	}
	copy(target[:], raw)
	return *target != [32]byte{}
}

func validAutomationHistoryPage(page automationport.AutomationHistoryQuery) bool {
	return page.Limit >= 1 && page.Limit <= 100 && page.Offset >= 0
}

func automationHistoryStoreError(err error) error {
	if errors.Is(err, automationport.ErrAutomationHistoryInvalid) || errors.Is(err, automationport.ErrAutomationHistoryConflict) || errors.Is(err, automationport.ErrAutomationHistoryUnavailable) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return automationport.ErrAutomationHistoryConflict
	}
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && postgres.Code == "23505" {
		return automationport.ErrAutomationHistoryConflict
	}
	return automationport.ErrAutomationHistoryUnavailable
}

func automationHistoryReaderError(err error) error {
	if errors.Is(err, automationport.ErrAutomationHistoryInvalid) {
		return automationport.ErrAutomationHistoryInvalid
	}
	return automationport.ErrAutomationHistoryUnavailable
}

func nilAutomationHistoryStoreDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Ptr && reflected.IsNil()
}
