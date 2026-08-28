package store

import (
	"context"
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

var audienceHistoryPostgresDSN = flag.String("audience-history-store-postgres-dsn", "", "PostgreSQL DSN for the optional 00114 audience history rollback test")

func TestAudienceHistoryStoreMapsAllCreatesAndGets(t *testing.T) {
	ctx := context.Background()
	group, packageValue, version, sender, rule, ruleVersion, definition, member := audienceHistoryFixtures()
	tx := &audienceHistoryTestTx{}
	store := &AudienceHistoryStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}

	group.ID = 101
	tx.row = audienceHistoryTestRow{values: audienceHistoryGroupValues(group)}
	gotGroup, err := store.CreateHistoricalAudienceGroup(ctx, withoutAudienceGroupID(group))
	assertAudienceHistoryResult(t, gotGroup, group, err, tx, "CreateHistoricalAudienceGroup", "segment_v1_audience_groups")
	tx.row = audienceHistoryTestRow{values: audienceHistoryGroupValues(group)}
	gotGroup, err = store.GetHistoricalAudienceGroup(ctx, group.ID)
	assertAudienceHistoryResult(t, gotGroup, group, err, tx, "GetHistoricalAudienceGroup", "segment_v1_audience_groups")

	packageValue.ID = 102
	tx.row = audienceHistoryTestRow{values: audienceHistoryPackageValues(packageValue)}
	gotPackage, err := store.CreateHistoricalAudiencePackage(ctx, withoutAudiencePackageID(packageValue))
	assertAudienceHistoryResult(t, gotPackage, packageValue, err, tx, "CreateHistoricalAudiencePackage", "segment_v1_audience_packages")
	if len(tx.args) != 23 || tx.args[2] != (pgtype.Int8{Int64: *packageValue.CurrentVersionSourceID, Valid: true}) {
		t.Fatalf("package SQLc arguments lost historical fields: %#v", tx.args)
	}
	tx.row = audienceHistoryTestRow{values: audienceHistoryPackageValues(packageValue)}
	gotPackage, err = store.GetHistoricalAudiencePackage(ctx, packageValue.ID)
	assertAudienceHistoryResult(t, gotPackage, packageValue, err, tx, "GetHistoricalAudiencePackage", "segment_v1_audience_packages")

	version.ID = 103
	tx.row = audienceHistoryTestRow{values: audienceHistoryVersionValues(version)}
	gotVersion, err := store.CreateHistoricalAudienceVersion(ctx, withoutAudienceVersionID(version))
	assertAudienceHistoryResult(t, gotVersion, version, err, tx, "CreateHistoricalAudienceVersion", "segment_v1_audience_versions")
	tx.row = audienceHistoryTestRow{values: audienceHistoryVersionValues(version)}
	gotVersion, err = store.GetHistoricalAudienceVersion(ctx, version.ID)
	assertAudienceHistoryResult(t, gotVersion, version, err, tx, "GetHistoricalAudienceVersion", "segment_v1_audience_versions")

	sender.ID = 104
	tx.row = audienceHistoryTestRow{values: audienceHistorySenderValues(sender)}
	gotSender, err := store.CreateHistoricalAudienceSender(ctx, withoutAudienceSenderID(sender))
	assertAudienceHistoryResult(t, gotSender, sender, err, tx, "CreateHistoricalAudienceSender", "segment_v1_audience_senders")
	tx.row = audienceHistoryTestRow{values: audienceHistorySenderValues(sender)}
	gotSender, err = store.GetHistoricalAudienceSender(ctx, sender.ID)
	assertAudienceHistoryResult(t, gotSender, sender, err, tx, "GetHistoricalAudienceSender", "segment_v1_audience_senders")

	rule.ID = 105
	tx.row = audienceHistoryTestRow{values: audienceHistoryRuleValues(rule)}
	gotRule, err := store.CreateHistoricalAudienceRule(ctx, withoutAudienceRuleID(rule))
	assertAudienceHistoryResult(t, gotRule, rule, err, tx, "CreateHistoricalAudienceRule", "segment_v1_audience_rules")
	if len(tx.args) != 9 {
		t.Fatalf("rule SQLc argument count = %d, want 9", len(tx.args))
	}
	tx.row = audienceHistoryTestRow{values: audienceHistoryRuleValues(rule)}
	gotRule, err = store.GetHistoricalAudienceRule(ctx, rule.ID)
	assertAudienceHistoryResult(t, gotRule, rule, err, tx, "GetHistoricalAudienceRule", "segment_v1_audience_rules")

	ruleVersion.ID = 106
	tx.row = audienceHistoryTestRow{values: audienceHistoryRuleVersionValues(ruleVersion)}
	gotRuleVersion, err := store.CreateHistoricalAudienceRuleVersion(ctx, withoutAudienceRuleVersionID(ruleVersion))
	assertAudienceHistoryResult(t, gotRuleVersion, ruleVersion, err, tx, "CreateHistoricalAudienceRuleVersion", "segment_v1_audience_rule_versions")
	tx.row = audienceHistoryTestRow{values: audienceHistoryRuleVersionValues(ruleVersion)}
	gotRuleVersion, err = store.GetHistoricalAudienceRuleVersion(ctx, ruleVersion.ID)
	assertAudienceHistoryResult(t, gotRuleVersion, ruleVersion, err, tx, "GetHistoricalAudienceRuleVersion", "segment_v1_audience_rule_versions")

	definition.ID = 107
	tx.row = audienceHistoryTestRow{values: audienceHistoryDefinitionValues(definition)}
	gotDefinition, err := store.CreateHistoricalAudienceDefinition(ctx, withoutAudienceDefinitionID(definition))
	assertAudienceHistoryResult(t, gotDefinition, definition, err, tx, "CreateHistoricalAudienceDefinition", "segment_v1_definitions")
	tx.row = audienceHistoryTestRow{values: audienceHistoryDefinitionValues(definition)}
	gotDefinition, err = store.GetHistoricalAudienceDefinition(ctx, definition.ID)
	assertAudienceHistoryResult(t, gotDefinition, definition, err, tx, "GetHistoricalAudienceDefinition", "segment_v1_definitions")

	member.ID = 108
	tx.row = audienceHistoryTestRow{values: audienceHistoryMemberValues(member)}
	gotMember, err := store.CreateHistoricalAudienceMember(ctx, withoutAudienceMemberID(member))
	assertAudienceHistoryResult(t, gotMember, member, err, tx, "CreateHistoricalAudienceMember", "segment_v1_audience_members")
	if len(tx.args) != 12 || !reflect.DeepEqual(tx.args[11], member.PayloadDigest[:]) {
		t.Fatalf("member SQLc arguments lost payload digest: %#v", tx.args)
	}
	tx.row = audienceHistoryTestRow{values: audienceHistoryMemberValues(member)}
	gotMember, err = store.GetHistoricalAudienceMember(ctx, member.ID)
	assertAudienceHistoryResult(t, gotMember, member, err, tx, "GetHistoricalAudienceMember", "segment_v1_audience_members")
}

func TestAudienceHistoryReaderMapsAllListsAndCounts(t *testing.T) {
	ctx := context.Background()
	group, packageValue, version, sender, rule, ruleVersion, definition, member := audienceHistoryFixtures()
	group.ID, packageValue.ID, version.ID, sender.ID = 101, 102, 103, 104
	rule.ID, ruleVersion.ID, definition.ID, member.ID = 105, 106, 107, 108
	tx := &audienceHistoryTestTx{row: audienceHistoryTestRow{values: []any{int64(1)}}}
	reader := &AudienceHistoryReader{db: tx}

	tests := []struct {
		name string
		call func() (int64, error)
		row  []any
	}{
		{"groups", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceGroups(ctx, 50, 0)
			return total, err
		}, audienceHistoryGroupValues(group)},
		{"packages", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudiencePackages(ctx, 50, 0)
			return total, err
		}, audienceHistoryPackageValues(packageValue)},
		{"versions", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceVersions(ctx, packageValue.ID, 50, 0)
			return total, err
		}, audienceHistoryVersionValues(version)},
		{"senders", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceSenders(ctx, packageValue.ID, 50, 0)
			return total, err
		}, audienceHistorySenderValues(sender)},
		{"rules", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceRules(ctx, 50, 0)
			return total, err
		}, audienceHistoryRuleValues(rule)},
		{"rule_versions", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceRuleVersions(ctx, rule.ID, 50, 0)
			return total, err
		}, audienceHistoryRuleVersionValues(ruleVersion)},
		{"definitions", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceDefinitions(ctx, 50, 0)
			return total, err
		}, audienceHistoryDefinitionValues(definition)},
		{"members", func() (int64, error) {
			_, total, err := reader.ListHistoricalAudienceMembers(ctx, packageValue.ID, 50, 0)
			return total, err
		}, audienceHistoryMemberValues(member)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx.row = audienceHistoryTestRow{values: []any{int64(1)}}
			tx.rows = &audienceHistoryTestRows{values: [][]any{test.row}}
			count, err := test.call()
			if err != nil || count != 1 || !strings.Contains(tx.query, "LIMIT $") || strings.Contains(tx.query, "segment_members") {
				t.Fatalf("list/count result = %d/%v query=%q", count, err, tx.query)
			}
		})
	}

	tx.row = audienceHistoryTestRow{values: audienceHistoryPackageValues(packageValue)}
	if got, err := reader.GetHistoricalAudiencePackage(ctx, packageValue.ID); err != nil || !reflect.DeepEqual(got, packageValue) {
		t.Fatalf("reader package get = %#v/%v", got, err)
	}
	tx.row = audienceHistoryTestRow{values: audienceHistoryDefinitionValues(definition)}
	if got, err := reader.GetHistoricalAudienceDefinition(ctx, definition.ID); err != nil || !reflect.DeepEqual(got, definition) {
		t.Fatalf("reader definition get = %#v/%v", got, err)
	}
}

func TestAudienceHistoryRejectsUnsafeInputsAndDatabaseShapes(t *testing.T) {
	_, packageValue, _, _, _, _, _, _ := audienceHistoryFixtures()
	store := &AudienceHistoryStore{tx: func(context.Context) (pgx.Tx, error) {
		t.Fatal("invalid input must not reach transaction")
		return nil, nil
	}}
	packageValue.ID = 1
	if _, err := store.CreateHistoricalAudiencePackage(context.Background(), packageValue); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) {
		t.Fatalf("invalid create = %v", err)
	}
	packageValue.ID = 0
	zero := time.Time{}
	packageValue.NextDailyAt = &zero
	if _, err := store.CreateHistoricalAudiencePackage(context.Background(), packageValue); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) {
		t.Fatalf("zero optional timestamp = %v", err)
	}
	if _, err := store.GetHistoricalAudiencePackage(context.Background(), 0); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) {
		t.Fatalf("invalid get = %v", err)
	}
	if _, _, err := (&AudienceHistoryReader{}).ListHistoricalAudienceGroups(context.Background(), 101, 0); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) {
		t.Fatalf("invalid page = %v", err)
	}
	if _, _, err := (&AudienceHistoryReader{}).ListHistoricalAudienceMembers(context.Background(), 0, 1, 0); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) {
		t.Fatalf("invalid parent = %v", err)
	}

	badDigest := segmentdb.SegmentV1Definition{ID: 1, SourceID: 1, CreatedAt: audienceHistoryTimestampValue(testAudienceHistoryTime()), UpdatedAt: audienceHistoryTimestampValue(testAudienceHistoryTime()), DefinitionDigest: []byte{1}}
	if _, err := audienceHistoryDefinition(badDigest); err == nil {
		t.Fatal("short digest row was accepted")
	}
	for _, cause := range []error{pgx.ErrNoRows, &pgconn.PgError{Code: "23505"}, &pgconn.PgError{Code: "23514"}, errors.New("private database error")} {
		tx := &audienceHistoryTestTx{row: audienceHistoryTestRow{err: cause}}
		store := &AudienceHistoryStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
		if _, err := store.CreateHistoricalAudienceGroup(context.Background(), withoutAudienceGroupID(audienceHistoryGroupFixture())); !errors.Is(err, audienceHistoryExpectedError(cause)) {
			t.Fatalf("database error %v = %v", cause, err)
		}
	}
}

// This is deliberately opt-in: parent integration runs it against the isolated
// V2 database after Goose 00114. It must never use a production DSN by default.
func TestAudienceHistoryPostgresRoundTripRollback(t *testing.T) {
	if *audienceHistoryPostgresDSN == "" {
		t.Skip("set -audience-history-store-postgres-dsn for 00114 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *audienceHistoryPostgresDSN)
	if err != nil {
		t.Fatalf("connect isolated audience-history database: %v", err)
	}
	defer pool.Close()

	group, packageValue, version, sender, rule, ruleVersion, definition, member := audienceHistoryFixtures()
	sender.StaffID = nil
	rule.OwnerStaffID = nil
	member.CustomerID = nil
	rollback := errors.New("force audience history rollback")
	var ids [8]int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewAudienceHistoryStore()
		createdGroup, err := store.CreateHistoricalAudienceGroup(txCtx, group)
		if err != nil {
			return err
		}
		packageValue.GroupHistoryID = &createdGroup.ID
		createdPackage, err := store.CreateHistoricalAudiencePackage(txCtx, packageValue)
		if err != nil {
			return err
		}
		version.PackageHistoryID, sender.PackageHistoryID, member.PackageHistoryID = createdPackage.ID, createdPackage.ID, createdPackage.ID
		createdVersion, err := store.CreateHistoricalAudienceVersion(txCtx, version)
		if err != nil {
			return err
		}
		createdSender, err := store.CreateHistoricalAudienceSender(txCtx, sender)
		if err != nil {
			return err
		}
		createdRule, err := store.CreateHistoricalAudienceRule(txCtx, rule)
		if err != nil {
			return err
		}
		ruleVersion.RuleHistoryID = createdRule.ID
		createdRuleVersion, err := store.CreateHistoricalAudienceRuleVersion(txCtx, ruleVersion)
		if err != nil {
			return err
		}
		createdDefinition, err := store.CreateHistoricalAudienceDefinition(txCtx, definition)
		if err != nil {
			return err
		}
		createdMember, err := store.CreateHistoricalAudienceMember(txCtx, member)
		if err != nil {
			return err
		}
		ids = [8]int64{createdGroup.ID, createdPackage.ID, createdVersion.ID, createdSender.ID, createdRule.ID, createdRuleVersion.ID, createdDefinition.ID, createdMember.ID}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		reader := NewAudienceHistoryReader(tx)
		readGroup, err := reader.GetHistoricalAudienceGroup(txCtx, ids[0])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readGroup, createdGroup)); err != nil {
			return err
		}
		readPackage, err := reader.GetHistoricalAudiencePackage(txCtx, ids[1])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readPackage, createdPackage)); err != nil {
			return err
		}
		readVersion, err := reader.GetHistoricalAudienceVersion(txCtx, ids[2])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readVersion, createdVersion)); err != nil {
			return err
		}
		readSender, err := reader.GetHistoricalAudienceSender(txCtx, ids[3])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readSender, createdSender)); err != nil {
			return err
		}
		readRule, err := reader.GetHistoricalAudienceRule(txCtx, ids[4])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readRule, createdRule)); err != nil {
			return err
		}
		readRuleVersion, err := reader.GetHistoricalAudienceRuleVersion(txCtx, ids[5])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readRuleVersion, createdRuleVersion)); err != nil {
			return err
		}
		readDefinition, err := reader.GetHistoricalAudienceDefinition(txCtx, ids[6])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readDefinition, createdDefinition)); err != nil {
			return err
		}
		readMember, err := reader.GetHistoricalAudienceMember(txCtx, ids[7])
		if err := audienceHistoryRoundTripError(err, audienceHistoryEquivalent(readMember, createdMember)); err != nil {
			return err
		}
		versions, total, err := reader.ListHistoricalAudienceVersions(txCtx, createdPackage.ID, 1, 0)
		if err != nil || total != 1 || len(versions) != 1 || !audienceHistoryEquivalent(versions[0], createdVersion) {
			return errors.New("historical version list/count did not round-trip")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("roundtrip transaction error = %v", err)
	}
	reader := NewAudienceHistoryReader(pool)
	checks := []func() error{
		func() error { _, err := reader.GetHistoricalAudienceGroup(ctx, ids[0]); return err },
		func() error { _, err := reader.GetHistoricalAudiencePackage(ctx, ids[1]); return err },
		func() error { _, err := reader.GetHistoricalAudienceVersion(ctx, ids[2]); return err },
		func() error { _, err := reader.GetHistoricalAudienceSender(ctx, ids[3]); return err },
		func() error { _, err := reader.GetHistoricalAudienceRule(ctx, ids[4]); return err },
		func() error { _, err := reader.GetHistoricalAudienceRuleVersion(ctx, ids[5]); return err },
		func() error { _, err := reader.GetHistoricalAudienceDefinition(ctx, ids[6]); return err },
		func() error { _, err := reader.GetHistoricalAudienceMember(ctx, ids[7]); return err },
	}
	for _, check := range checks {
		if err := check(); !errors.Is(err, segmentport.ErrAudienceHistoryConflict) {
			t.Fatalf("rollback left historical row behind: %v", err)
		}
	}
}

func audienceHistoryExpectedError(cause error) error {
	var postgres *pgconn.PgError
	if errors.Is(cause, pgx.ErrNoRows) || errors.As(cause, &postgres) && strings.HasPrefix(postgres.Code, "23") {
		return segmentport.ErrAudienceHistoryConflict
	}
	return segmentport.ErrAudienceHistoryUnavailable
}

func audienceHistoryRoundTripError(err error, equal bool) error {
	if err != nil {
		return err
	}
	if !equal {
		return errors.New("historical field round-trip mismatch")
	}
	return nil
}

func audienceHistoryEquivalent(left, right any) bool {
	return audienceHistoryValueEquivalent(reflect.ValueOf(left), reflect.ValueOf(right))
}

func audienceHistoryValueEquivalent(left, right reflect.Value) bool {
	if !left.IsValid() || !right.IsValid() {
		return left.IsValid() == right.IsValid()
	}
	if left.Type() != right.Type() {
		return false
	}
	if left.Type() == reflect.TypeOf(time.Time{}) {
		leftTime := left.Interface().(time.Time).UTC().Truncate(time.Microsecond)
		rightTime := right.Interface().(time.Time).UTC().Truncate(time.Microsecond)
		return leftTime.Equal(rightTime)
	}
	switch left.Kind() {
	case reflect.Pointer:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() == right.IsNil()
		}
		return audienceHistoryValueEquivalent(left.Elem(), right.Elem())
	case reflect.Struct:
		for index := 0; index < left.NumField(); index++ {
			if !audienceHistoryValueEquivalent(left.Field(index), right.Field(index)) {
				return false
			}
		}
		return true
	case reflect.Array:
		for index := 0; index < left.Len(); index++ {
			if !audienceHistoryValueEquivalent(left.Index(index), right.Index(index)) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left.Interface(), right.Interface())
	}
}

func assertAudienceHistoryResult[T any](t *testing.T, got, want T, err error, tx *audienceHistoryTestTx, name, table string) {
	t.Helper()
	if err != nil || !reflect.DeepEqual(got, want) || !strings.Contains(tx.query, "-- name: "+name) || !strings.Contains(tx.query, table) || strings.Contains(tx.query, "operation_receipt") || strings.Contains(tx.query, "event") || strings.Contains(tx.query, "river") {
		t.Fatalf("%s result=%#v err=%v query=%q", name, got, err, tx.query)
	}
}

func testAudienceHistoryTime() time.Time {
	return time.Date(2026, 8, 28, 15, 4, 5, 123456000, time.FixedZone("V1", 8*3600))
}
func testAudienceHistoryDigest(seed byte) [32]byte {
	var value [32]byte
	for index := range value {
		value[index] = seed
	}
	return value
}
func audienceHistoryFixtures() (segmentport.HistoricalAudienceGroup, segmentport.HistoricalAudiencePackage, segmentport.HistoricalAudienceVersion, segmentport.HistoricalAudienceSender, segmentport.HistoricalAudienceRule, segmentport.HistoricalAudienceRuleVersion, segmentport.HistoricalAudienceDefinition, segmentport.HistoricalAudienceMember) {
	stamp := testAudienceHistoryTime()
	later := stamp.Add(time.Hour)
	groupID, currentVersion, templateVersion, staffID, customerID := int64(11), int64(12), int64(-4), int64(21), int64(31)
	group := segmentport.HistoricalAudienceGroup{SourceID: 1, Name: "历史分组", CreatedAt: stamp, UpdatedAt: later}
	packageValue := segmentport.HistoricalAudiencePackage{SourceID: 2, GroupHistoryID: &groupID, CurrentVersionSourceID: &currentVersion, PackageKey: "history-package", Name: "历史包", NaturalLanguageDefinition: "历史自然语言", OriginalStatus: "active", QueryMode: "legacy", IdentityPolicy: "unionid", IncrementalEnabled: true, DailyEnabled: true, IncrementalIntervalSecs: -30, DailyRefreshTime: "08:00", Timezone: "Asia/Shanghai", LookbackSecs: -1, LastIncrementalAt: &stamp, LastDailyRefreshedAt: &later, NextIncrementalAt: &stamp, NextDailyAt: &later, PausedReason: "", CreatedAt: stamp, UpdatedAt: later, RuntimeDigest: testAudienceHistoryDigest(2)}
	version := segmentport.HistoricalAudienceVersion{SourceID: 3, PackageHistoryID: 102, VersionNumber: -1, OriginalStatus: "published", AIPrompt: "prompt", AIRationale: "reason", NaturalLanguageExplanation: "explanation", CreatedAt: stamp, PublishedAt: &later, TemplateKey: "tpl", TemplateVersion: &templateVersion, TemplateFingerprint: "fingerprint", DefinitionDigest: testAudienceHistoryDigest(3)}
	sender := segmentport.HistoricalAudienceSender{SourceID: 4, PackageHistoryID: 102, StaffID: &staffID, DisplayName: "发送人", Priority: -7, OriginalStatus: "disabled", CreatedAt: stamp, UpdatedAt: later}
	rule := segmentport.HistoricalAudienceRule{SourceID: 5, RuleKey: "legacy-rule", DisplayName: "规则", Description: "描述", RuleType: "legacy", OwnerStaffID: &staffID, OriginalStatus: "inactive", CreatedAt: stamp, UpdatedAt: later}
	ruleVersion := segmentport.HistoricalAudienceRuleVersion{SourceID: 6, RuleHistoryID: 105, Version: -2, ExecutorType: "legacy", OriginalStatus: "archived", PublishedAt: &later, CreatedAt: stamp, DefinitionDigest: testAudienceHistoryDigest(6)}
	definition := segmentport.HistoricalAudienceDefinition{SourceID: 7, Code: "legacy-code", DisplayName: "定义", Description: "描述", SourceType: "sql", SQLDialect: "postgres", OriginalStatus: "active", Version: -3, CachedHeadcount: -8, LastRefreshedAt: &later, UsageCount: -9, CreatedAt: stamp, UpdatedAt: later, DefinitionDigest: testAudienceHistoryDigest(7)}
	member := segmentport.HistoricalAudienceMember{SourceID: 8, PackageHistoryID: 102, CustomerID: &customerID, IdentityKind: "unionid", OriginalStatus: "exited", FirstEnteredAt: stamp, LastSeenAt: later, LastUpdatedAt: later, ExitedAt: &later, CreatedAt: stamp, UpdatedAt: later, PayloadDigest: testAudienceHistoryDigest(8)}
	return group, packageValue, version, sender, rule, ruleVersion, definition, member
}

func audienceHistoryGroupFixture() segmentport.HistoricalAudienceGroup {
	group, _, _, _, _, _, _, _ := audienceHistoryFixtures()
	return group
}
func withoutAudienceGroupID(value segmentport.HistoricalAudienceGroup) segmentport.HistoricalAudienceGroup {
	value.ID = 0
	return value
}
func withoutAudiencePackageID(value segmentport.HistoricalAudiencePackage) segmentport.HistoricalAudiencePackage {
	value.ID = 0
	return value
}
func withoutAudienceVersionID(value segmentport.HistoricalAudienceVersion) segmentport.HistoricalAudienceVersion {
	value.ID = 0
	return value
}
func withoutAudienceSenderID(value segmentport.HistoricalAudienceSender) segmentport.HistoricalAudienceSender {
	value.ID = 0
	return value
}
func withoutAudienceRuleID(value segmentport.HistoricalAudienceRule) segmentport.HistoricalAudienceRule {
	value.ID = 0
	return value
}
func withoutAudienceRuleVersionID(value segmentport.HistoricalAudienceRuleVersion) segmentport.HistoricalAudienceRuleVersion {
	value.ID = 0
	return value
}
func withoutAudienceDefinitionID(value segmentport.HistoricalAudienceDefinition) segmentport.HistoricalAudienceDefinition {
	value.ID = 0
	return value
}
func withoutAudienceMemberID(value segmentport.HistoricalAudienceMember) segmentport.HistoricalAudienceMember {
	value.ID = 0
	return value
}

func audienceHistoryTimestampValue(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func audienceHistoryOptionalTimestampValue(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return audienceHistoryTimestampValue(*value)
}
func audienceHistoryOptionalInt64Value(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func audienceHistoryGroupValues(value segmentport.HistoricalAudienceGroup) []any {
	return []any{value.ID, value.SourceID, value.Name, audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryTimestampValue(value.UpdatedAt)}
}
func audienceHistoryPackageValues(value segmentport.HistoricalAudiencePackage) []any {
	return []any{value.ID, value.SourceID, audienceHistoryOptionalInt64Value(value.GroupHistoryID), audienceHistoryOptionalInt64Value(value.CurrentVersionSourceID), value.PackageKey, value.Name, value.NaturalLanguageDefinition, value.OriginalStatus, value.QueryMode, value.IdentityPolicy, value.IncrementalEnabled, value.DailyEnabled, value.IncrementalIntervalSecs, value.DailyRefreshTime, value.Timezone, value.LookbackSecs, audienceHistoryOptionalTimestampValue(value.LastIncrementalAt), audienceHistoryOptionalTimestampValue(value.LastDailyRefreshedAt), audienceHistoryOptionalTimestampValue(value.NextIncrementalAt), audienceHistoryOptionalTimestampValue(value.NextDailyAt), value.PausedReason, audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryTimestampValue(value.UpdatedAt), value.RuntimeDigest[:]}
}
func audienceHistoryVersionValues(value segmentport.HistoricalAudienceVersion) []any {
	return []any{value.ID, value.SourceID, value.PackageHistoryID, value.VersionNumber, value.OriginalStatus, value.AIPrompt, value.AIRationale, value.NaturalLanguageExplanation, audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryOptionalTimestampValue(value.PublishedAt), value.TemplateKey, audienceHistoryOptionalInt64Value(value.TemplateVersion), value.TemplateFingerprint, value.DefinitionDigest[:]}
}
func audienceHistorySenderValues(value segmentport.HistoricalAudienceSender) []any {
	return []any{value.ID, value.SourceID, value.PackageHistoryID, audienceHistoryOptionalInt64Value(value.StaffID), value.DisplayName, value.Priority, value.OriginalStatus, audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryTimestampValue(value.UpdatedAt)}
}
func audienceHistoryRuleValues(value segmentport.HistoricalAudienceRule) []any {
	return []any{value.ID, value.SourceID, value.RuleKey, value.DisplayName, value.Description, value.RuleType, audienceHistoryOptionalInt64Value(value.OwnerStaffID), value.OriginalStatus, audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryTimestampValue(value.UpdatedAt)}
}
func audienceHistoryRuleVersionValues(value segmentport.HistoricalAudienceRuleVersion) []any {
	return []any{value.ID, value.SourceID, value.RuleHistoryID, value.Version, value.ExecutorType, value.OriginalStatus, audienceHistoryOptionalTimestampValue(value.PublishedAt), audienceHistoryTimestampValue(value.CreatedAt), value.DefinitionDigest[:]}
}
func audienceHistoryDefinitionValues(value segmentport.HistoricalAudienceDefinition) []any {
	return []any{value.ID, value.SourceID, value.Code, value.DisplayName, value.Description, value.SourceType, value.SQLDialect, value.OriginalStatus, value.Version, value.CachedHeadcount, audienceHistoryOptionalTimestampValue(value.LastRefreshedAt), value.UsageCount, audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryTimestampValue(value.UpdatedAt), value.DefinitionDigest[:]}
}
func audienceHistoryMemberValues(value segmentport.HistoricalAudienceMember) []any {
	return []any{value.ID, value.SourceID, value.PackageHistoryID, audienceHistoryOptionalInt64Value(value.CustomerID), value.IdentityKind, value.OriginalStatus, audienceHistoryTimestampValue(value.FirstEnteredAt), audienceHistoryTimestampValue(value.LastSeenAt), audienceHistoryTimestampValue(value.LastUpdatedAt), audienceHistoryOptionalTimestampValue(value.ExitedAt), audienceHistoryTimestampValue(value.CreatedAt), audienceHistoryTimestampValue(value.UpdatedAt), value.PayloadDigest[:]}
}

type audienceHistoryTestTx struct {
	pgx.Tx
	row   audienceHistoryTestRow
	rows  *audienceHistoryTestRows
	query string
	args  []any
}

func (tx *audienceHistoryTestTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	tx.query, tx.args = query, append([]any(nil), args...)
	return tx.row
}
func (tx *audienceHistoryTestTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	tx.query, tx.args = query, append([]any(nil), args...)
	if tx.rows == nil {
		return &audienceHistoryTestRows{}, nil
	}
	return tx.rows, nil
}

type audienceHistoryTestRow struct {
	values []any
	err    error
}

func (row audienceHistoryTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("unexpected scan destination count")
	}
	for index, value := range row.values {
		reflect.ValueOf(dest[index]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

type audienceHistoryTestRows struct {
	pgx.Rows
	values [][]any
	index  int
	err    error
}

func (rows *audienceHistoryTestRows) Close()     {}
func (rows *audienceHistoryTestRows) Err() error { return rows.err }
func (rows *audienceHistoryTestRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}
func (rows *audienceHistoryTestRows) Scan(dest ...any) error {
	return (audienceHistoryTestRow{values: rows.values[rows.index-1]}).Scan(dest...)
}
