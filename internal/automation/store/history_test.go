package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var automationHistoryTestDatabaseURL = flag.String("automation-history-test-database-url", "", "PostgreSQL URL for Automation V1 history rollback test")

func TestAutomationHistoryRowsPreserveHistoricalValues(t *testing.T) {
	at := time.Date(2026, 8, 28, 14, 15, 16, 123456000, time.FixedZone("v1", 8*3600))
	key, payload, first, second := automationHistoryDigestBytes(1), automationHistoryDigestBytes(2), automationHistoryDigestBytes(3), automationHistoryDigestBytes(4)
	sop, err := automationHistorySOP(automationdb.AutomationV1SopHistory{ID: 1, SourceID: 2, SourceKeyDigest: key, SourcePayloadDigest: payload, PoolKey: "pool", DayIndex: -3, ContentMasked: "text\n", ImagesDigest: first, OriginalEnabled: true, CreatedAt: historyStamp(at), UpdatedAt: historyStamp(at.Add(-time.Hour))})
	if err != nil || sop.DayIndex != -3 || !sop.UpdatedAt.Before(sop.CreatedAt) || sop.ImagesDigest != [32]byte{3} {
		t.Fatalf("sop = %#v, %v", sop, err)
	}
	config, err := automationHistoryConfig(automationdb.AutomationV1AgentConfigHistory{ID: 2, SourceID: 3, SourceKeyDigest: key, SourcePayloadDigest: payload, AgentCode: "a", DisplayName: "", ScenarioCode: "", DraftVersion: -1, PublishedVersion: -2, PublishedAt: "legacy", LastModifiedAt: "", LastModifiedSource: "", SubmittedAt: "", CreatedAt: historyStamp(at), UpdatedAt: historyStamp(at.Add(-time.Hour)), ActorsDigest: first, ConfigDigest: second})
	if err != nil || config.DraftVersion != -1 || config.PublishedAt != "legacy" || config.ActorsDigest != [32]byte{3} {
		t.Fatalf("config = %#v, %v", config, err)
	}
	prompt, err := automationHistoryPrompt(automationdb.AutomationV1PromptHistory{ID: 3, SourceID: 4, SourceKeyDigest: key, SourcePayloadDigest: payload, AgentCode: "a", DisplayName: "", Version: -5, CreatedAt: historyStamp(at), UpdatedAt: historyStamp(at.Add(-time.Hour)), PromptDigest: first})
	if err != nil || prompt.Version != -5 || prompt.PromptDigest != [32]byte{3} {
		t.Fatalf("prompt = %#v, %v", prompt, err)
	}
	agent, err := automationHistoryAgent(automationdb.AutomationV1AgentHistory{ID: 4, SourceID: 5, SourceKeyDigest: key, SourcePayloadDigest: payload, ProgramSourceID: 0, WorkflowSourceID: -1, NodeSourceID: 0, TaskSourceID: -2, AgentCode: "a", AgentName: "", OriginalType: "legacy", OriginalStatus: "", SortOrder: -3, ArchivedAt: "not-time", CreatedAt: historyStamp(at), UpdatedAt: historyStamp(at.Add(-time.Hour)), ActorsDigest: first, ConfigurationDigest: second})
	if err != nil || agent.WorkflowSourceID != -1 || agent.SortOrder != -3 || agent.ArchivedAt != "not-time" || agent.ActorsDigest != [32]byte{3} {
		t.Fatalf("agent = %#v, %v", agent, err)
	}

	bad := automationdb.AutomationV1SopHistory{ID: 1, SourceID: 2, SourceKeyDigest: key, SourcePayloadDigest: payload, ImagesDigest: []byte{1}, CreatedAt: historyStamp(at), UpdatedAt: historyStamp(at)}
	if _, err = automationHistorySOP(bad); !errors.Is(err, automationport.ErrAutomationHistoryUnavailable) {
		t.Fatalf("bad stored digest = %v", err)
	}
}

func TestAutomationHistoryReaderRejectsInvalidInput(t *testing.T) {
	reader := NewAutomationHistoryReader(nil)
	if _, err := reader.GetHistoricalAutomationSOP(context.Background(), 0); !errors.Is(err, automationport.ErrAutomationHistoryInvalid) {
		t.Fatal(err)
	}
	if values, _, err := reader.ListHistoricalAutomationAgents(context.Background(), automationport.AutomationHistoryQuery{Limit: 0}); !errors.Is(err, automationport.ErrAutomationHistoryInvalid) || values == nil {
		t.Fatalf("invalid page: %#v %v", values, err)
	}
	if _, err := reader.GetHistoricalAutomationPrompt(context.Background(), 1); !errors.Is(err, automationport.ErrAutomationHistoryUnavailable) {
		t.Fatal(err)
	}
}

func TestAutomationHistoryPostgreSQLRoundTripRollback(t *testing.T) {
	if *automationHistoryTestDatabaseURL == "" {
		t.Skip("-automation-history-test-database-url is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *automationHistoryTestDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	uow, repository := platformstore.NewUnitOfWork(pool), NewRepository(pool)
	seed := time.Now().UnixNano()
	sop, config, prompt, agent := automationHistoryStoreFacts(seed)
	before, err := automationHistoryCounts(ctx, automationdb.New(pool))
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Within(ctx, func(tx context.Context) error {
		createdSOP, err := repository.CreateHistoricalAutomationSOP(tx, sop)
		if err != nil {
			return err
		}
		createdConfig, err := repository.CreateHistoricalAutomationConfig(tx, config)
		if err != nil {
			return err
		}
		createdPrompt, err := repository.CreateHistoricalAutomationPrompt(tx, prompt)
		if err != nil {
			return err
		}
		createdAgent, err := repository.CreateHistoricalAutomationAgent(tx, agent)
		if err != nil {
			return err
		}
		db, err := platformstore.TxFromContext(tx)
		if err != nil {
			return err
		}
		reader := NewAutomationHistoryReader(db)
		if got, err := repository.GetHistoricalAutomationSOP(tx, createdSOP.ID); err != nil || digestSOP(got) != digestSOP(createdSOP) {
			return fmt.Errorf("get sop: %w", err)
		}
		if got, err := repository.GetHistoricalAutomationConfig(tx, createdConfig.ID); err != nil || digestConfig(got) != digestConfig(createdConfig) {
			return fmt.Errorf("get config: %w", err)
		}
		if got, err := repository.GetHistoricalAutomationPrompt(tx, createdPrompt.ID); err != nil || digestPrompt(got) != digestPrompt(createdPrompt) {
			return fmt.Errorf("get prompt: %w", err)
		}
		if got, err := repository.GetHistoricalAutomationAgent(tx, createdAgent.ID); err != nil || digestAgent(got) != digestAgent(createdAgent) {
			return fmt.Errorf("get agent: %w", err)
		}
		page := automationport.AutomationHistoryQuery{Limit: 10, Offset: 0}
		if values, total, err := reader.ListHistoricalAutomationSOPs(tx, page); err != nil || total != before.sop+1 || !hasHistorySOP(values, createdSOP.ID) {
			return fmt.Errorf("list sop: %d/%w", total, err)
		}
		if values, total, err := reader.ListHistoricalAutomationConfigs(tx, page); err != nil || total != before.config+1 || !hasHistoryConfig(values, createdConfig.ID) {
			return fmt.Errorf("list config: %d/%w", total, err)
		}
		if values, total, err := reader.ListHistoricalAutomationPrompts(tx, page); err != nil || total != before.prompt+1 || !hasHistoryPrompt(values, createdPrompt.ID) {
			return fmt.Errorf("list prompt: %d/%w", total, err)
		}
		if values, total, err := reader.ListHistoricalAutomationAgents(tx, page); err != nil || total != before.agent+1 || !hasHistoryAgent(values, createdAgent.ID) {
			return fmt.Errorf("list agent: %d/%w", total, err)
		}
		return errAutomationHistoryRollback
	})
	if !errors.Is(err, errAutomationHistoryRollback) {
		t.Fatalf("round trip = %v", err)
	}
	after, err := automationHistoryCounts(ctx, automationdb.New(pool))
	if err != nil || after != before {
		t.Fatalf("rollback counts = %#v/%#v/%v", before, after, err)
	}
}

type automationHistoryCount struct{ sop, config, prompt, agent int64 }

var errAutomationHistoryRollback = errors.New("rollback automation history")

func automationHistoryCounts(ctx context.Context, q *automationdb.Queries) (automationHistoryCount, error) {
	var counts automationHistoryCount
	var err error
	if counts.sop, err = q.CountHistoricalAutomationSOPs(ctx); err != nil {
		return counts, err
	}
	if counts.config, err = q.CountHistoricalAutomationConfigs(ctx); err != nil {
		return counts, err
	}
	if counts.prompt, err = q.CountHistoricalAutomationPrompts(ctx); err != nil {
		return counts, err
	}
	counts.agent, err = q.CountHistoricalAutomationAgents(ctx)
	return counts, err
}

func automationHistoryStoreFacts(seed int64) (automationport.HistoricalAutomationSOP, automationport.HistoricalAutomationConfig, automationport.HistoricalAutomationPrompt, automationport.HistoricalAutomationAgent) {
	at := time.Date(2026, 8, 28, 15, 0, 0, 123456000, time.UTC)
	identity := func(id int64, value byte) automationport.HistoricalAutomationIdentity {
		return automationport.HistoricalAutomationIdentity{SourceID: id, SourceKeyDigest: [32]byte{value}, SourcePayloadDigest: [32]byte{value + 10}}
	}
	return automationport.HistoricalAutomationSOP{HistoricalAutomationIdentity: identity(seed+1, 1), PoolKey: fmt.Sprintf("sop-%d", seed), ContentMasked: "text", ImagesDigest: [32]byte{21}, CreatedAt: at, UpdatedAt: at},
		automationport.HistoricalAutomationConfig{HistoricalAutomationIdentity: identity(seed+2, 2), AgentCode: fmt.Sprintf("config-%d", seed), ActorsDigest: [32]byte{22}, ConfigDigest: [32]byte{23}, CreatedAt: at, UpdatedAt: at},
		automationport.HistoricalAutomationPrompt{HistoricalAutomationIdentity: identity(seed+3, 3), AgentCode: fmt.Sprintf("prompt-%d", seed), PromptDigest: [32]byte{24}, CreatedAt: at, UpdatedAt: at},
		automationport.HistoricalAutomationAgent{HistoricalAutomationIdentity: identity(seed+4, 4), AgentCode: fmt.Sprintf("agent-%d", seed), ActorsDigest: [32]byte{25}, ConfigurationDigest: [32]byte{26}, CreatedAt: at, UpdatedAt: at}
}

func automationHistoryDigestBytes(value byte) []byte {
	digest := [32]byte{value}
	return digest[:]
}
func historyStamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func digestSOP(value automationport.HistoricalAutomationSOP) [32]byte {
	digest, _ := automationapp.HistoricalAutomationSOPDigest(value)
	return digest
}
func digestConfig(value automationport.HistoricalAutomationConfig) [32]byte {
	digest, _ := automationapp.HistoricalAutomationConfigDigest(value)
	return digest
}
func digestPrompt(value automationport.HistoricalAutomationPrompt) [32]byte {
	digest, _ := automationapp.HistoricalAutomationPromptDigest(value)
	return digest
}
func digestAgent(value automationport.HistoricalAutomationAgent) [32]byte {
	digest, _ := automationapp.HistoricalAutomationAgentDigest(value)
	return digest
}
func hasHistorySOP(values []automationport.HistoricalAutomationSOP, id int64) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}
func hasHistoryConfig(values []automationport.HistoricalAutomationConfig, id int64) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}
func hasHistoryPrompt(values []automationport.HistoricalAutomationPrompt, id int64) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}
func hasHistoryAgent(values []automationport.HistoricalAutomationAgent, id int64) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}
