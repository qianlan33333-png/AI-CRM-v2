package automation_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var p4AutomationAgentsExpectedWaterline = flag.Int("expected-waterline", 44, "expected current migration waterline for the Automation Agents storage catalog")

func TestP4AutomationAgentsABNormalIdempotencyAndNoExecution(t *testing.T) {
	pool, ctx := openPool(t)
	service := newP4AutomationAgentService(pool)
	code, actor := p4AutomationAgentCode(t), p4AutomationActor()
	create := automationport.CreateCommand{Actor: actor, IdempotencyKey: p4AutomationKey(code, "create"), Agent: automationport.Agent{AgentName: "P4 Automation Agent", AgentCode: code, AutomationType: automationport.AutomationTypeAgent, Status: automationport.AgentStatusActive}}
	created, err := service.Create(ctx, create)
	if err != nil || created.ID < 1 || created.DraftRolePrompt != "" || created.DraftTaskPrompt != "" || created.PublishedVersion != 1 {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	replayed, err := service.Create(ctx, create)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("create replay=%+v err=%v", replayed, err)
	}
	name, role := "P4 Automation Agent Updated", "role v2"
	updated, err := service.Update(ctx, automationport.UpdateCommand{ID: created.ID, AgentName: &name, RolePrompt: &role, Actor: actor, IdempotencyKey: p4AutomationKey(code, "update")})
	if err != nil || updated.DraftVersion != 2 || updated.AgentName != name {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	noChange, err := service.Update(ctx, automationport.UpdateCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "empty-update")})
	if err != nil || noChange.ID != created.ID || noChange.DraftVersion != updated.DraftVersion {
		t.Fatalf("empty update=%+v err=%v", noChange, err)
	}
	published, err := service.Publish(ctx, automationport.MutationCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "publish")})
	if err != nil || published.PublishedVersion != published.DraftVersion {
		t.Fatalf("publish=%+v err=%v", published, err)
	}
	fixed, err := service.SaveFixedContent(ctx, automationport.FixedContentCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "content"), ContentPackage: automationport.FixedContentPackage{ImageLibraryIDs: []int64{4, 4}}})
	if err != nil || len(fixed.FixedContentPackage.ImageLibraryIDs) != 1 || fixed.FixedContentPackage.ImageLibraryIDs[0] != 4 {
		t.Fatalf("fixed=%+v err=%v", fixed, err)
	}
	emptied, err := service.SaveFixedContent(ctx, automationport.FixedContentCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "empty-content")})
	if err != nil || emptied.FixedContentPackage.ContentText != "" || len(emptied.FixedContentPackage.ImageLibraryIDs) != 0 {
		t.Fatalf("empty fixed content=%+v err=%v", emptied, err)
	}
	copied, err := service.Copy(ctx, automationport.MutationCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "copy")})
	if err != nil || copied.AgentCode != code+"_copy_001" || copied.ID == created.ID {
		t.Fatalf("copy=%+v err=%v", copied, err)
	}
	paused, err := service.SetStatus(ctx, automationport.MutationCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "pause")}, automationport.AgentStatusPaused)
	if err != nil || paused.Status != automationport.AgentStatusPaused {
		t.Fatalf("pause=%+v err=%v", paused, err)
	}
	active, err := service.SetStatus(ctx, automationport.MutationCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "activate")}, automationport.AgentStatusActive)
	if err != nil || active.Status != automationport.AgentStatusActive {
		t.Fatalf("activate=%+v err=%v", active, err)
	}
	archived, err := service.SetStatus(ctx, automationport.MutationCommand{ID: created.ID, Actor: actor, IdempotencyKey: p4AutomationKey(code, "archive")}, automationport.AgentStatusArchived)
	if err != nil || archived.Status != automationport.AgentStatusArchived {
		t.Fatalf("archive=%+v err=%v", archived, err)
	}
	if _, err = service.Get(ctx, created.ID); !errors.Is(err, automationapp.ErrAgentNotFound) {
		t.Fatalf("archived Get error=%v", err)
	}
	page, err := service.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == created.ID {
			t.Fatal("archived item visible")
		}
	}
	var configurations, receipts, completed, events, deliveries int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM automation_agent_configurations WHERE id=$1),(SELECT count(*) FROM automation_agent_operation_receipts WHERE actor_scope=$2),(SELECT count(*) FROM automation_agent_operation_receipts WHERE actor_scope=$2 AND state='completed')`, created.ID, fmt.Sprintf("admin:%d", actor)).Scan(&configurations, &receipts, &completed)
	if err != nil {
		t.Fatal(err)
	}
	eventKeys := p4AutomationEventKeys(code)
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM event_log WHERE idempotency_key = ANY($1)),(SELECT count(*) FROM event_deliveries d JOIN event_log e ON e.id=d.event_id WHERE e.idempotency_key = ANY($1))`, eventKeys).Scan(&events, &deliveries)
	if err != nil {
		t.Fatal(err)
	}
	if configurations != 1 || receipts != 10 || completed != 10 || events != 8 || deliveries != 0 {
		t.Fatalf("configuration/receipt/completed/event/delivery=%d/%d/%d/%d/%d", configurations, receipts, completed, events, deliveries)
	}
}

func TestP4AutomationAgentsABBoundaryErrorAndUOWRollback(t *testing.T) {
	pool, ctx := openPool(t)
	service := newP4AutomationAgentService(pool)
	code := p4AutomationAgentCode(t)
	if _, err := service.Create(ctx, automationport.CreateCommand{Actor: 702, IdempotencyKey: "p4-automation-agents-invalid-0001", Agent: automationport.Agent{AgentName: "invalid", AgentCode: "bad code", AutomationType: automationport.AutomationTypeAgent, Status: automationport.AgentStatusActive}}); !errors.Is(err, automationapp.ErrInvalidAgent) {
		t.Fatalf("invalid code error=%v", err)
	}
	archived, err := service.Create(ctx, automationport.CreateCommand{Actor: 702, IdempotencyKey: p4AutomationKey(code, "archived-create"), Agent: automationport.Agent{AgentName: "archived", AgentCode: code + "archived", AutomationType: automationport.AutomationTypeFixedScript, Status: automationport.AgentStatusArchived}})
	if err != nil || archived.ID < 1 || archived.Status != automationport.AgentStatusArchived {
		t.Fatalf("archived create=%+v err=%v", archived, err)
	}
	valid, err := service.Create(ctx, automationport.CreateCommand{Actor: 702, IdempotencyKey: p4AutomationKey(code, "valid"), Agent: automationport.Agent{AgentName: "valid", AgentCode: code + "a", AutomationType: automationport.AutomationTypeAgent, Status: automationport.AgentStatusActive}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveFixedContent(ctx, automationport.FixedContentCommand{ID: valid.ID, Actor: 702, IdempotencyKey: "p4-automation-agents-overflow-001", ContentPackage: automationport.FixedContentPackage{ImageLibraryIDs: []int64{1, 2, 3, 4}}}); !errors.Is(err, automationapp.ErrInvalidAgent) {
		t.Fatalf("over-limit fixed content error=%v", err)
	}
	if _, err = service.Create(ctx, automationport.CreateCommand{Actor: 702, IdempotencyKey: p4AutomationKey(code, "valid"), Agent: automationport.Agent{AgentName: "different", AgentCode: code + "b", AutomationType: automationport.AutomationTypeAgent, Status: automationport.AgentStatusActive}}); !errors.Is(err, automationapp.ErrAgentConflict) {
		t.Fatalf("idempotency conflict error=%v", err)
	}
	rollbackCode := p4AutomationAgentCode(t)
	failing := automationapp.NewAgentService(platformstore.NewUnitOfWork(pool), automationstore.NewAgentRepository(), failingAutomationAppender{})
	if _, err = failing.Create(ctx, automationport.CreateCommand{Actor: 703, IdempotencyKey: p4AutomationKey(rollbackCode, "rollback"), Agent: automationport.Agent{AgentName: "rollback", AgentCode: rollbackCode, AutomationType: automationport.AutomationTypeAgent, Status: automationport.AgentStatusActive}}); !errors.Is(err, automationapp.ErrAgentUnavailable) {
		t.Fatalf("failing UoW create error=%v", err)
	}
	var configurations, receipts, events int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM automation_agent_configurations WHERE agent_code=$1),(SELECT count(*) FROM automation_agent_operation_receipts WHERE actor_scope='admin:703'),(SELECT count(*) FROM event_log WHERE payload->>'agent_id' IN (SELECT id::text FROM automation_agent_configurations WHERE agent_code=$1))`, rollbackCode).Scan(&configurations, &receipts, &events)
	if err != nil {
		t.Fatal(err)
	}
	if configurations != 0 || receipts != 0 || events != 0 {
		t.Fatalf("rollback configuration/receipt/event=%d/%d/%d", configurations, receipts, events)
	}
}

func TestP4AutomationAgentsABStorageHasNoTenantOrCrossDomainOwnership(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, invalidConstraints, foreignKeys, tenantColumns, indexes int
	err := pool.QueryRow(ctx, `SELECT (SELECT max(version_id) FROM goose_db_version WHERE is_applied),(SELECT count(*) FROM pg_constraint WHERE conrelid IN ('automation_agent_configurations'::regclass,'automation_agent_operation_receipts'::regclass) AND NOT convalidated),(SELECT count(*) FROM pg_constraint WHERE conrelid IN ('automation_agent_configurations'::regclass,'automation_agent_operation_receipts'::regclass) AND contype='f'),(SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('automation_agent_configurations','automation_agent_operation_receipts') AND column_name ILIKE '%tenant%'),(SELECT count(*) FROM pg_index WHERE indrelid IN ('automation_agent_configurations'::regclass,'automation_agent_operation_receipts'::regclass) AND indisvalid AND indisready AND indislive)`).Scan(&waterline, &invalidConstraints, &foreignKeys, &tenantColumns, &indexes)
	if err != nil {
		t.Fatal(err)
	}
	if waterline != *p4AutomationAgentsExpectedWaterline || invalidConstraints != 0 || foreignKeys != 0 || tenantColumns != 0 || indexes != 6 {
		t.Fatalf("waterline/invalid/fks/tenant/indexes=%d/%d/%d/%d/%d", waterline, invalidConstraints, foreignKeys, tenantColumns, indexes)
	}
}

func newP4AutomationAgentService(pool *pgxpool.Pool) *automationapp.Service {
	return automationapp.NewAgentService(platformstore.NewUnitOfWork(pool), automationstore.NewAgentRepository(), eventstore.NewAppender())
}

var errP4AutomationAppender = errors.New("forced event append failure")

type failingAutomationAppender struct{}

func (failingAutomationAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errP4AutomationAppender
}
func p4AutomationAgentCode(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("p4ab_%d", time.Now().UnixNano())
}
func p4AutomationKey(code, operation string) string { return "p4-automation-" + code + "-" + operation }
func p4AutomationEventKeys(code string) []string {
	return []string{p4AutomationEventKey("create", p4AutomationKey(code, "create")), p4AutomationEventKey("update", p4AutomationKey(code, "update")), p4AutomationEventKey("publish", p4AutomationKey(code, "publish")), p4AutomationEventKey("fixed_content", p4AutomationKey(code, "content")), p4AutomationEventKey("fixed_content", p4AutomationKey(code, "empty-content")), p4AutomationEventKey("set_status", p4AutomationKey(code, "pause")), p4AutomationEventKey("set_status", p4AutomationKey(code, "activate")), p4AutomationEventKey("set_status", p4AutomationKey(code, "archive"))}
}
func p4AutomationEventKey(operation, key string) string {
	digest := sha256.Sum256([]byte("automation.agent." + operation + "\x00" + key))
	return "automation.agent." + operation + ":" + hex.EncodeToString(digest[:])
}
func p4AutomationActor() int64 { return time.Now().UnixNano()%1_000_000_000 + 1 }
