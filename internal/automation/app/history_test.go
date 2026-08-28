package app

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

type automationHistoryFixture struct {
	sop    automationport.HistoricalAutomationSOP
	config automationport.HistoricalAutomationConfig
	prompt automationport.HistoricalAutomationPrompt
	agent  automationport.HistoricalAutomationAgent
}

func newAutomationHistoryFixture() automationHistoryFixture {
	at := time.Date(2026, 8, 28, 12, 13, 14, 987654321, time.FixedZone("v1", 8*3600))
	identity := func(sourceID int64, seed byte) automationport.HistoricalAutomationIdentity {
		return automationport.HistoricalAutomationIdentity{SourceID: sourceID, SourceKeyDigest: [32]byte{seed}, SourcePayloadDigest: [32]byte{seed + 20}}
	}
	return automationHistoryFixture{
		sop:    automationport.HistoricalAutomationSOP{HistoricalAutomationIdentity: identity(1, 1), PoolKey: " pool ", DayIndex: -3, ContentMasked: "保留\n空白", ImagesDigest: [32]byte{2}, OriginalEnabled: true, CreatedAt: at, UpdatedAt: at.Add(-time.Hour)},
		config: automationport.HistoricalAutomationConfig{HistoricalAutomationIdentity: identity(2, 2), AgentCode: "agent", DisplayName: "显示名", ScenarioCode: "scenario", OriginalEnabled: false, DraftVersion: -1, PublishedVersion: -2, PublishedAt: "unparseable legacy", LastModifiedAt: "", LastModifiedSource: "source", SubmittedForPublish: true, SubmittedAt: "", CreatedAt: at, UpdatedAt: at.Add(-time.Hour), ActorsDigest: [32]byte{3}, ConfigDigest: [32]byte{4}},
		prompt: automationport.HistoricalAutomationPrompt{HistoricalAutomationIdentity: identity(3, 3), AgentCode: "agent", DisplayName: "显示名", OriginalEnabled: true, Version: -9, CreatedAt: at, UpdatedAt: at.Add(-time.Hour), PromptDigest: [32]byte{5}},
		agent:  automationport.HistoricalAutomationAgent{HistoricalAutomationIdentity: identity(4, 4), ProgramSourceID: 0, WorkflowSourceID: -1, NodeSourceID: 0, TaskSourceID: -7, AgentCode: "agent", AgentName: "名称", OriginalType: "legacy", OriginalStatus: "unknown", SortOrder: -2, OriginalEnabled: false, CreatedAt: at, UpdatedAt: at.Add(-time.Hour), ArchivedAt: "not an instant", ActorsDigest: [32]byte{6}, ConfigurationDigest: [32]byte{7}},
	}
}

func (f automationHistoryFixture) source(kind string) string {
	switch kind {
	case automationport.AutomationHistorySOP:
		return automationHistorySource(f.sop.SourceKeyDigest)
	case automationport.AutomationHistoryConfig:
		return automationHistorySource(f.config.SourceKeyDigest)
	case automationport.AutomationHistoryPrompt:
		return automationHistorySource(f.prompt.SourceKeyDigest)
	default:
		return automationHistorySource(f.agent.SourceKeyDigest)
	}
}

func automationHistorySource(digest [32]byte) string { return hex.EncodeToString(digest[:]) }

func (f automationHistoryFixture) importFact(writer *AutomationHistoryWriter, ctx context.Context, kind, source string) (automationport.AutomationHistoryReceipt, error) {
	switch kind {
	case automationport.AutomationHistorySOP:
		return writer.ImportSOP(ctx, source, f.sop)
	case automationport.AutomationHistoryConfig:
		return writer.ImportConfig(ctx, source, f.config)
	case automationport.AutomationHistoryPrompt:
		return writer.ImportPrompt(ctx, source, f.prompt)
	default:
		return writer.ImportAgent(ctx, source, f.agent)
	}
}

type automationHistoryStore struct {
	ctx            context.Context
	sop            automationport.HistoricalAutomationSOP
	config         automationport.HistoricalAutomationConfig
	prompt         automationport.HistoricalAutomationPrompt
	agent          automationport.HistoricalAutomationAgent
	creates, reads int
	err            error
	mutate         bool
}

func (s *automationHistoryStore) check(ctx context.Context) {
	if ctx != s.ctx {
		panic("caller transaction was not preserved")
	}
}
func (s *automationHistoryStore) CreateHistoricalAutomationSOP(ctx context.Context, value automationport.HistoricalAutomationSOP) (automationport.HistoricalAutomationSOP, error) {
	s.check(ctx)
	s.creates++
	value.ID = 101
	if s.mutate {
		value.DayIndex++
	}
	s.sop = value
	return value, s.err
}
func (s *automationHistoryStore) GetHistoricalAutomationSOP(ctx context.Context, _ int64) (automationport.HistoricalAutomationSOP, error) {
	s.check(ctx)
	s.reads++
	return s.sop, s.err
}
func (s *automationHistoryStore) CreateHistoricalAutomationConfig(ctx context.Context, value automationport.HistoricalAutomationConfig) (automationport.HistoricalAutomationConfig, error) {
	s.check(ctx)
	s.creates++
	value.ID = 102
	if s.mutate {
		value.ScenarioCode += " changed"
	}
	s.config = value
	return value, s.err
}
func (s *automationHistoryStore) GetHistoricalAutomationConfig(ctx context.Context, _ int64) (automationport.HistoricalAutomationConfig, error) {
	s.check(ctx)
	s.reads++
	return s.config, s.err
}
func (s *automationHistoryStore) CreateHistoricalAutomationPrompt(ctx context.Context, value automationport.HistoricalAutomationPrompt) (automationport.HistoricalAutomationPrompt, error) {
	s.check(ctx)
	s.creates++
	value.ID = 103
	if s.mutate {
		value.Version++
	}
	s.prompt = value
	return value, s.err
}
func (s *automationHistoryStore) GetHistoricalAutomationPrompt(ctx context.Context, _ int64) (automationport.HistoricalAutomationPrompt, error) {
	s.check(ctx)
	s.reads++
	return s.prompt, s.err
}
func (s *automationHistoryStore) CreateHistoricalAutomationAgent(ctx context.Context, value automationport.HistoricalAutomationAgent) (automationport.HistoricalAutomationAgent, error) {
	s.check(ctx)
	s.creates++
	value.ID = 104
	if s.mutate {
		value.OriginalStatus += " changed"
	}
	s.agent = value
	return value, s.err
}
func (s *automationHistoryStore) GetHistoricalAutomationAgent(ctx context.Context, _ int64) (automationport.HistoricalAutomationAgent, error) {
	s.check(ctx)
	s.reads++
	return s.agent, s.err
}

type automationHistoryJournal struct {
	ctx            context.Context
	receipts       map[string]automationport.AutomationHistoryReceipt
	loads, records int
	loadErr        error
	recordErr      error
}

func (j *automationHistoryJournal) LoadAutomationHistory(ctx context.Context, kind, source string) (automationport.AutomationHistoryReceipt, bool, error) {
	if ctx != j.ctx {
		panic("journal caller transaction was not preserved")
	}
	j.loads++
	receipt, found := j.receipts[kind+"/"+source]
	return receipt, found, j.loadErr
}
func (j *automationHistoryJournal) RecordAutomationHistory(ctx context.Context, receipt automationport.AutomationHistoryReceipt) error {
	if ctx != j.ctx {
		panic("journal caller transaction was not preserved")
	}
	j.records++
	if j.recordErr != nil {
		return j.recordErr
	}
	j.receipts[receipt.Kind+"/"+receipt.SourceIdentifier] = receipt
	return nil
}

func newAutomationHistoryWriterTest(t *testing.T) (*AutomationHistoryWriter, *automationHistoryStore, *automationHistoryJournal, context.Context) {
	t.Helper()
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "caller-tx")
	store := &automationHistoryStore{ctx: ctx}
	journal := &automationHistoryJournal{ctx: ctx, receipts: make(map[string]automationport.AutomationHistoryReceipt)}
	writer, err := NewAutomationHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	return writer, store, journal, ctx
}

func TestAutomationHistoryWriterCreateReplayAndTargetDrift(t *testing.T) {
	for _, kind := range []string{automationport.AutomationHistorySOP, automationport.AutomationHistoryConfig, automationport.AutomationHistoryPrompt, automationport.AutomationHistoryAgent} {
		t.Run(kind, func(t *testing.T) {
			writer, store, journal, ctx := newAutomationHistoryWriterTest(t)
			fixture := newAutomationHistoryFixture()
			first, err := fixture.importFact(writer, ctx, kind, fixture.source(kind))
			if err != nil || first.TargetID < 1 || first.TargetDigest == [32]byte{} || first.Replayed || journal.records != 1 {
				t.Fatalf("first: %+v %v", first, err)
			}
			replay, err := fixture.importFact(writer, ctx, kind, fixture.source(kind))
			if err != nil || !replay.Replayed || replay.TargetID != first.TargetID || replay.TargetDigest != first.TargetDigest || replay.PayloadDigest != first.PayloadDigest || store.creates != 1 || store.reads != 1 {
				t.Fatalf("replay: %+v %v", replay, err)
			}
			switch kind {
			case automationport.AutomationHistorySOP:
				store.sop.ContentMasked += " changed"
			case automationport.AutomationHistoryConfig:
				store.config.AgentCode += " changed"
			case automationport.AutomationHistoryPrompt:
				store.prompt.DisplayName += " changed"
			case automationport.AutomationHistoryAgent:
				store.agent.AgentName += " changed"
			}
			if _, err = fixture.importFact(writer, ctx, kind, fixture.source(kind)); !errors.Is(err, automationport.ErrAutomationHistoryConflict) {
				t.Fatalf("target drift accepted: %v", err)
			}
		})
	}
}

func TestAutomationHistoryWriterPreservesLegacyFacts(t *testing.T) {
	writer, store, _, ctx := newAutomationHistoryWriterTest(t)
	f := newAutomationHistoryFixture()
	for _, kind := range []string{automationport.AutomationHistorySOP, automationport.AutomationHistoryConfig, automationport.AutomationHistoryPrompt, automationport.AutomationHistoryAgent} {
		if _, err := f.importFact(writer, ctx, kind, f.source(kind)); err != nil {
			t.Fatal(err)
		}
	}
	if store.sop.DayIndex != -3 || !store.sop.UpdatedAt.Before(store.sop.CreatedAt) || store.config.DraftVersion != -1 || store.config.PublishedAt != "unparseable legacy" || store.prompt.Version != -9 || store.agent.ProgramSourceID != 0 || store.agent.WorkflowSourceID != -1 || store.agent.SortOrder != -2 || store.agent.ArchivedAt != "not an instant" || store.agent.CreatedAt.Location() != time.UTC || store.agent.CreatedAt.Nanosecond() != 987654000 {
		t.Fatal("historical facts were normalized beyond PostgreSQL timestamps")
	}
}

func TestAutomationHistoryWriterRejectsInvalidBeforeAdapters(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		edit       func(*automationHistoryFixture)
	}{
		{"sop source id", automationport.AutomationHistorySOP, func(f *automationHistoryFixture) { f.sop.SourceID = 0 }},
		{"sop digest", automationport.AutomationHistorySOP, func(f *automationHistoryFixture) { f.sop.ImagesDigest = [32]byte{} }},
		{"config nul", automationport.AutomationHistoryConfig, func(f *automationHistoryFixture) { f.config.AgentCode = "x\x00" }},
		{"config payload", automationport.AutomationHistoryConfig, func(f *automationHistoryFixture) { f.config.SourcePayloadDigest = [32]byte{} }},
		{"prompt utf8", automationport.AutomationHistoryPrompt, func(f *automationHistoryFixture) { f.prompt.DisplayName = string([]byte{255}) }},
		{"agent target", automationport.AutomationHistoryAgent, func(f *automationHistoryFixture) { f.agent.ID = 1 }},
		{"agent timestamp", automationport.AutomationHistoryAgent, func(f *automationHistoryFixture) { f.agent.CreatedAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer, store, journal, ctx := newAutomationHistoryWriterTest(t)
			f := newAutomationHistoryFixture()
			tc.edit(&f)
			_, err := f.importFact(writer, ctx, tc.kind, f.source(tc.kind))
			if !errors.Is(err, automationport.ErrAutomationHistoryInvalid) || store.creates != 0 || journal.loads != 0 {
				t.Fatalf("invalid reached adapters: %v", err)
			}
		})
	}
	writer, store, journal, ctx := newAutomationHistoryWriterTest(t)
	f := newAutomationHistoryFixture()
	if _, err := f.importFact(writer, ctx, automationport.AutomationHistorySOP, "not-the-source-key"); !errors.Is(err, automationport.ErrAutomationHistoryInvalid) || store.creates != 0 || journal.loads != 0 {
		t.Fatalf("invalid source accepted: %v", err)
	}
}

func TestAutomationHistoryWriterErrorsFailClosed(t *testing.T) {
	for _, point := range []string{"load", "create", "record", "bad-create", "replay-read"} {
		t.Run(point, func(t *testing.T) {
			writer, store, journal, ctx := newAutomationHistoryWriterTest(t)
			f := newAutomationHistoryFixture()
			private := errors.New("private source text")
			want := automationport.ErrAutomationHistoryUnavailable
			switch point {
			case "load":
				journal.loadErr = private
			case "create":
				store.err = private
			case "record":
				journal.recordErr = private
			case "bad-create":
				store.mutate, want = true, automationport.ErrAutomationHistoryConflict
			case "replay-read":
				if _, err := f.importFact(writer, ctx, automationport.AutomationHistorySOP, f.source(automationport.AutomationHistorySOP)); err != nil {
					t.Fatal(err)
				}
				store.err = private
			}
			got, err := f.importFact(writer, ctx, automationport.AutomationHistorySOP, f.source(automationport.AutomationHistorySOP))
			if !errors.Is(err, want) || got != (automationport.AutomationHistoryReceipt{}) {
				t.Fatalf("success leaked: %+v %v", got, err)
			}
		})
	}
}

func TestAutomationHistoryDigestRejectsInvalidStoredTarget(t *testing.T) {
	f := newAutomationHistoryFixture()
	f.sop.ID = 101
	if _, err := HistoricalAutomationSOPDigest(f.sop); err != nil {
		t.Fatal(err)
	}
	f.sop.ID = 0
	if _, err := HistoricalAutomationSOPDigest(f.sop); !errors.Is(err, automationport.ErrAutomationHistoryInvalid) {
		t.Fatal(err)
	}
	var store *automationHistoryStore
	var journal *automationHistoryJournal
	if _, err := NewAutomationHistoryWriter(store, journal); !errors.Is(err, automationport.ErrAutomationHistoryUnavailable) {
		t.Fatal(err)
	}
}
