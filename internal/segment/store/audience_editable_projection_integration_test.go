package store

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	authfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store/acceptancefixture"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/acceptancefixture"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
)

func TestAudienceEditableProjectionPostgres(t *testing.T) {
	databaseURL := os.Getenv("AICRM_SEGMENT_EDITABLE_PROJECTION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AICRM_SEGMENT_EDITABLE_PROJECTION_DATABASE_URL for PostgreSQL projection test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	at := time.Date(2026, 8, 30, 4, 0, 0, 0, time.UTC)
	var actorID, customerOne, customerTwo, historyGroup, historyPackage, hxcHistoryPackage int64
	actorID, err = authfixture.CreateAdminUser(ctx, pool, "audience-editable-projection-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `
INSERT INTO segment_v1_audience_packages
  (source_id,group_history_id,current_version_source_id,package_key,name,natural_language_definition,
   original_status,query_mode,identity_policy,incremental_enabled,daily_enabled,
   incremental_interval_seconds,daily_refresh_time,timezone,lookback_seconds,paused_reason,
   created_at,updated_at,runtime_digest)
VALUES (31,$1,NULL,'audience_huangxiaocan_active_not_ai_opc_not_paid','HXC deferred package','HXC deferred',
        'active','full_sql','external_userid',false,false,180,'08:00','Asia/Shanghai',86400,'',$2,$2,decode(repeat('33',32),'hex'))
RETURNING id`, historyGroup, at).Scan(&hxcHistoryPackage); err != nil {
		t.Fatal(err)
	}
	if customerOne, err = contactfixture.CreateCustomer(ctx, pool, "audience-projection-one"); err != nil {
		t.Fatal(err)
	}
	if customerTwo, err = contactfixture.CreateCustomer(ctx, pool, "audience-projection-two"); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO segment_v1_audience_groups(source_id,name,created_at,updated_at) VALUES(10,'V1 active group',$1,$1) RETURNING id`, at).Scan(&historyGroup); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `
INSERT INTO segment_v1_audience_packages
  (source_id,group_history_id,current_version_source_id,package_key,name,natural_language_definition,
   original_status,query_mode,identity_policy,incremental_enabled,daily_enabled,
   incremental_interval_seconds,daily_refresh_time,timezone,lookback_seconds,paused_reason,
   created_at,updated_at,runtime_digest)
VALUES (14,$1,NULL,'v1-active','V1 active package','submitted questionnaire','active','incremental_event',
        'external_userid',true,false,180,'08:00','Asia/Shanghai',86400,'',$2,$2,decode(repeat('11',32),'hex'))
RETURNING id`, historyGroup, at).Scan(&historyPackage); err != nil {
		t.Fatal(err)
	}
	for index, customerID := range []int64{customerOne, customerTwo} {
		if _, err = pool.Exec(ctx, `
INSERT INTO segment_v1_audience_members
  (source_id,package_history_id,customer_id,identity_kind,original_status,first_entered_at,last_seen_at,last_updated_at,created_at,updated_at,payload_digest)
VALUES ($1,$2,$3,'external_userid','active',$4,$4,$4,$4,$4,decode(repeat('22',32),'hex'))`, index+1, historyPackage, customerID, at); err != nil {
			t.Fatal(err)
		}
	}
	uow := platformstore.NewUnitOfWork(pool)
	service := segmentapp.NewAudienceEditableProjectionService(uow, NewAudienceEditableProjectionStore())
	first, err := service.Project(ctx, actorID, at)
	if err != nil {
		t.Fatal(err)
	}
	if first.GroupsCreated != 1 || first.PackagesCreated != 1 || first.MembersProjected != 2 || first.HistoryOnlyPreserved != 1 {
		t.Fatalf("first projection = %#v", first)
	}
	second, err := service.Project(ctx, actorID, at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second.GroupsReplayed != 1 || second.PackagesReplayed != 1 || second.MembersProjected != 0 {
		t.Fatalf("replay = %#v", second)
	}
	var segmentID, memberCount int64
	var lifecycle, definition string
	if err = pool.QueryRow(ctx, `
SELECT segment.id, metadata.lifecycle, segment.definition::text, segment.member_count
FROM ai_audience_v1_editable_package_projections AS projection
JOIN segments AS segment ON segment.id=projection.segment_id
JOIN ai_audience_package_metadata AS metadata ON metadata.segment_id=segment.id
WHERE projection.package_history_id=$1`, historyPackage).Scan(&segmentID, &lifecycle, &definition, &memberCount); err != nil {
		t.Fatal(err)
	}
	if segmentID < 1 || lifecycle != "paused" || definition != `{"op": "eq", "field": "legacy_audience_package_source_id", "value": 14}` || memberCount != 2 {
		t.Fatalf("projected package = %d %s %s %d", segmentID, lifecycle, definition, memberCount)
	}
	var hxcProjectionCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM ai_audience_v1_editable_package_projections WHERE package_history_id=$1`, hxcHistoryPackage).Scan(&hxcProjectionCount); err != nil || hxcProjectionCount != 0 {
		t.Fatalf("HXC deferred projection = %d, error = %v", hxcProjectionCount, err)
	}
	if err = uow.Within(ctx, func(tx context.Context) error {
		database, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		ids, queryErr := NewQuerySet(database).LegacyAudiencePackageSnapshot(tx, 14)
		if queryErr != nil {
			return queryErr
		}
		if !reflect.DeepEqual(ids, []int64{customerOne, customerTwo}) {
			t.Fatalf("snapshot IDs = %v", ids)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
