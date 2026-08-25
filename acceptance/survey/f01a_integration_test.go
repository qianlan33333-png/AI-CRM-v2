package survey_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 survey migration database")

func TestF01AMigrationHistoryFixture(t *testing.T) {
	pool, ctx := openPool(t)
	marker := unique("migration-history")
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, err := eventstore.NewAppender().Append(tx, eventport.Event{Type: "f01a.survey.migration_fixture", Payload: json.RawMessage(`{"fixture":true}`), OccurredAt: time.Now().UTC(), IdempotencyKey: marker})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestF01ACreateReplayActorIsolationAndReadsShareRealUoW(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	key := unique("create-key")
	command := questionnaireCommand(5101, key, unique("questionnaire"))
	created, err := service.Create(ctx, command)
	if err != nil || created.ID < 1 || len(created.Questions) != 2 || created.Questions[0].ID < 1 || created.Questions[0].Options[0].ID < 1 {
		t.Fatalf("create=%#v err=%v", created, err)
	}
	replay, err := service.Create(ctx, command)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	changed := command
	changed.Title = "不同载荷"
	if _, err = service.Create(ctx, changed); !errors.Is(err, surveyapp.ErrConflict) {
		t.Fatalf("payload conflict=%v", err)
	}
	other := command
	other.Actor, other.Name, other.Title, other.Slug = 5102, unique("actor"), "另一问卷", unique("actor-slug")
	otherCreated, err := service.Create(ctx, other)
	if err != nil || otherCreated.ID == created.ID {
		t.Fatalf("actor isolation=%#v err=%v", otherCreated, err)
	}
	page, err := service.ListLegacy(ctx, 50, 0)
	if err != nil || page.Total < 2 {
		t.Fatalf("list=%#v err=%v", page, err)
	}
	detail, err := service.Get(ctx, created.ID)
	if err != nil || detail.ID != created.ID || detail.Questions[0].Options[0].OptionText != "增长" {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	assertFacts(t, pool, ctx, command.Actor, key, command.Name, 1, 1, 1)
	assertFacts(t, pool, ctx, other.Actor, key, other.Name, 1, 1, 1)
}

func TestF01AEventConflictRollsBackQuestionnaireChildrenReceiptAndCounter(t *testing.T) {
	pool, ctx := openPool(t)
	actor, key, name := int64(5201), unique("rollback-key"), unique("rollback-name")
	eventKey := surveyEventKey(actor, key)
	var counterBefore int64
	if err := pool.QueryRow(ctx, `SELECT total_questionnaires FROM questionnaire_catalog_counters WHERE singleton`).Scan(&counterBefore); err != nil {
		t.Fatal(err)
	}
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, appendErr := eventstore.NewAppender().Append(tx, eventport.Event{Type: "f01a.conflict.fixture", Payload: json.RawMessage(`{"fixture":true}`), OccurredAt: time.Now().UTC(), IdempotencyKey: eventKey})
		return appendErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = realService(pool).Create(ctx, questionnaireCommand(actor, key, name)); !errors.Is(err, surveyapp.ErrUnavailable) {
		t.Fatalf("event conflict=%v", err)
	}
	assertFacts(t, pool, ctx, actor, key, name, 0, 0, 1)
	var questionnaires, questions, options int
	var counterAfter int64
	if err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM questionnaires WHERE created_by=$1 AND name=$2),
      (SELECT count(*) FROM questionnaire_questions qq JOIN questionnaires q ON q.id=qq.questionnaire_id WHERE q.created_by=$1 AND q.name=$2),
      (SELECT count(*) FROM questionnaire_options qo JOIN questionnaire_questions qq ON qq.id=qo.question_id JOIN questionnaires q ON q.id=qq.questionnaire_id WHERE q.created_by=$1 AND q.name=$2),
      (SELECT total_questionnaires FROM questionnaire_catalog_counters WHERE singleton)`, actor, name).Scan(&questionnaires, &questions, &options, &counterAfter); err != nil || questionnaires != 0 || questions != 0 || options != 0 || counterAfter != counterBefore {
		t.Fatalf("rollback facts=%d/%d/%d counter=%d/%d err=%v", questionnaires, questions, options, counterBefore, counterAfter, err)
	}
}

func TestF01AS200KReceiptLookupUsesActorBusinessKeyIndex(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO questionnaire_operation_receipts
      (operation,actor_scope,key_digest,payload_digest,state,result_snapshot,created_at,completed_at)
      SELECT 'create','admin:'||g,decode(md5('key-'||g)||md5('key2-'||g),'hex'),
             decode(md5('payload-'||g)||md5('payload2-'||g),'hex'),'completed',jsonb_build_object('id',g),now(),now()
      FROM generate_series(2000000,2199999) AS g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE questionnaire_operation_receipts`); err != nil {
		t.Fatal(err)
	}
	plan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF)
      SELECT id,state,result_snapshot FROM questionnaire_operation_receipts
      WHERE operation='create' AND actor_scope='admin:2100000'
        AND key_digest=decode(md5('key-2100000')||md5('key2-2100000'),'hex')`)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) || !strings.Contains(plan, `"Node Type": "Index Scan"`) || !strings.Contains(plan, `questionnaire_operation_recei_operation_actor_scope_key_dig_key`) {
		t.Fatalf("illegal S200K plan:\n%s", plan)
	}
}

func TestF01AStorageCatalogSingleInstanceAndOwnership(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, invalidConstraints, invalidIndexes, tenantColumns, eventFKs int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('questionnaires'::regclass,'questionnaire_questions'::regclass,'questionnaire_options'::regclass,'questionnaire_operation_receipts'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('questionnaires'::regclass,'questionnaire_questions'::regclass,'questionnaire_options'::regclass,'questionnaire_operation_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'questionnaire%' AND column_name ~* 'tenant|workspace|organization'),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('questionnaires'::regclass,'questionnaire_questions'::regclass,'questionnaire_options'::regclass,'questionnaire_operation_receipts'::regclass) AND confrelid='event_log'::regclass)`).Scan(&waterline, &invalidConstraints, &invalidIndexes, &tenantColumns, &eventFKs)
	if err != nil || waterline != 31 || invalidConstraints != 0 || invalidIndexes != 0 || tenantColumns != 0 || eventFKs != 0 {
		t.Fatalf("catalog=%d/%d/%d/%d/%d err=%v", waterline, invalidConstraints, invalidIndexes, tenantColumns, eventFKs, err)
	}
}

func TestF01BManagementUpdateDisableDeleteAndDuplicateShareRealUoW(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	actor := int64(5401)
	created, err := service.Create(ctx, questionnaireCommand(actor, unique("f01b-create"), unique("f01b-name")))
	if err != nil {
		t.Fatal(err)
	}
	update := surveyport.UpdateCommand{Questionnaire: created, Actor: actor, IdempotencyKey: unique("f01b-update")}
	update.Title = "F01B 更新"
	updated, err := service.Update(ctx, created.ID, update)
	if err != nil || updated.ID != created.ID || updated.Version != created.Version+1 || updated.Title != update.Title {
		t.Fatalf("update=%#v err=%v", updated, err)
	}
	replay, err := service.Update(ctx, created.ID, update)
	if err != nil || replay.ID != updated.ID || replay.Version != updated.Version {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	disabled, err := service.SetDisabled(ctx, created.ID, true, actor, unique("f01b-disable"))
	if err != nil || !disabled.IsDisabled {
		t.Fatalf("disabled=%#v err=%v", disabled, err)
	}
	deleted, err := service.Delete(ctx, created.ID, actor, unique("f01b-delete"))
	if err != nil || !deleted.Deleted || deleted.Questionnaire.ID != created.ID {
		t.Fatalf("deleted=%#v err=%v", deleted, err)
	}
	if _, err = service.Get(ctx, created.ID); !errors.Is(err, surveyapp.ErrNotFound) {
		t.Fatalf("deleted definition still readable: %v", err)
	}

	source, err := service.Create(ctx, questionnaireCommand(actor, unique("f01b-copy-source"), unique("f01b-copy-name")))
	if err != nil {
		t.Fatal(err)
	}
	copy, err := service.Duplicate(ctx, source.ID, actor, unique("f01b-copy"), "", "")
	if err != nil || copy.ID == source.ID || !copy.IsDisabled || copy.Title != source.Title+" Copy" || len(copy.Questions) != len(source.Questions) {
		t.Fatalf("copy=%#v source=%#v err=%v", copy, source, err)
	}
	var updateReceipts, disableReceipts, deleteReceipts, updateEvents, deleteEvents int
	err = pool.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM questionnaire_management_receipts WHERE operation='update'),
	  (SELECT count(*) FROM questionnaire_management_receipts WHERE operation='disable'),
	  (SELECT count(*) FROM questionnaire_management_receipts WHERE operation='delete'),
      (SELECT count(*) FROM event_log WHERE event_type='survey.updated'),
      (SELECT count(*) FROM event_log WHERE event_type='survey.deleted')`).Scan(&updateReceipts, &disableReceipts, &deleteReceipts, &updateEvents, &deleteEvents)
	if err != nil || updateReceipts < 1 || disableReceipts < 1 || deleteReceipts < 1 || updateEvents < 2 || deleteEvents < 1 {
		t.Fatalf("management facts=%d/%d/%d/%d/%d err=%v", updateReceipts, disableReceipts, deleteReceipts, updateEvents, deleteEvents, err)
	}
}

func questionnaireCommand(actor int64, key, name string) surveyport.CreateCommand {
	return surveyport.CreateCommand{Questionnaire: surveyport.Questionnaire{
		Name: name, Title: name, Description: "F01A", Slug: unique("slug"), AnswerDisplayMode: surveyport.AllInOne,
		AssessmentConfig: json.RawMessage(`{}`), ScoreRules: []surveyport.ScoreRule{},
		Questions: []surveyport.Question{
			{Type: surveyport.SingleChoice, Title: "目标", Required: true, SortOrder: 0, Options: []surveyport.Option{{OptionText: "增长", TagCodes: []string{}, SortOrder: 0}, {OptionText: "交付", TagCodes: []string{}, SortOrder: 1}}},
			{Type: surveyport.Mobile, Title: "手机号", Required: true, SortOrder: 1, Options: []surveyport.Option{}},
		},
	}, Actor: actor, IdempotencyKey: key}
}

func realService(pool *pgxpool.Pool) *surveyapp.Service {
	return surveyapp.NewService(platformstore.NewUnitOfWork(pool), surveystore.NewQuestionnaireRepository(), eventstore.NewAppender())
}

func assertFacts(t *testing.T, pool *pgxpool.Pool, ctx context.Context, actor int64, key, name string, questionnaires, receipts, events int) {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(key))
	var gotQuestionnaires, gotReceipts, gotEvents int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM questionnaires WHERE created_by=$1 AND name=$2),
      (SELECT count(*) FROM questionnaire_operation_receipts WHERE actor_scope=$3 AND key_digest=$4),
      (SELECT count(*) FROM event_log WHERE idempotency_key=$5)`, actor, name, fmt.Sprintf("admin:%d", actor), keyDigest[:], surveyEventKey(actor, key)).Scan(&gotQuestionnaires, &gotReceipts, &gotEvents)
	if err != nil || gotQuestionnaires != questionnaires || gotReceipts != receipts || gotEvents != events {
		t.Fatalf("facts=%d/%d/%d err=%v want=%d/%d/%d", gotQuestionnaires, gotReceipts, gotEvents, err, questionnaires, receipts, events)
	}
}

func surveyEventKey(actor int64, key string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, key)))
	return "survey.create:" + hex.EncodeToString(digest[:])
}

func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, acceptancefixtures.F01ADatabaseName); err != nil {
		if f01abErr := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, acceptancefixtures.F01ABDatabaseName); f01abErr != nil {
			if surveyPushErr := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, acceptancefixtures.SurveyPushDatabaseName); surveyPushErr != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	return pool, ctx
}

func explain(t *testing.T, ctx context.Context, tx pgx.Tx, query string) string {
	t.Helper()
	rows, err := tx.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func unique(prefix string) string { return fmt.Sprintf("f01a-%s-%d", prefix, time.Now().UnixNano()) }
