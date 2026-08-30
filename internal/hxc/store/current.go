package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CurrentRepository struct{ pool *pgxpool.Pool }

func NewCurrentRepository(pool *pgxpool.Pool) *CurrentRepository {
	return &CurrentRepository{pool: pool}
}

func (repository *CurrentRepository) Replace(ctx context.Context, rows []hxcport.Current, summary hxcport.SyncSummary, now time.Time) error {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	queries := hxcdb.New(tx)
	if err := queries.ClearHXCCurrentMatches(ctx); err != nil {
		return err
	}
	ids := make([]string, len(rows))
	for index, row := range rows {
		ids[index] = row.HXCUserID
		capabilityUsage, err := json.Marshal(row.CapabilityUsage)
		if err != nil {
			return err
		}
		focusTopics, err := json.Marshal(row.FocusTopics)
		if err != nil {
			return err
		}
		customerID := pgtype.Int8{}
		if row.CustomerID > 0 {
			customerID = pgtype.Int8{Int64: int64(row.CustomerID), Valid: true}
		}
		if err := queries.UpsertHXCCurrent(ctx, hxcdb.UpsertHXCCurrentParams{
			HxcUserID: row.HXCUserID, CustomerID: customerID, MatchState: string(row.MatchState), SubscriptionTier: row.SubscriptionTier,
			SubscriptionExpiresAt: pts(row.SubscriptionExpiresAt), MonthlyChatQuota: row.MonthlyChatQuota, CurrentPeriodUsed: row.CurrentPeriodUsed,
			ConsultationLimit: row.ConsultationLimit, ConsultationUsed: row.ConsultationUsed,
			Sessions7d: row.Sessions7D, Sessions30d: row.Sessions30D, SessionsTotal: row.SessionsTotal,
			UserMessages7d: row.UserMessages7D, UserMessages30d: row.UserMessages30D, UserMessagesTotal: row.UserMessagesTotal,
			CapabilityUsage: capabilityUsage, LastUsedAt: pts(row.LastUsedAt), LastCapability: text(row.LastCapability),
			BusinessStage: text(row.BusinessStage), MainLineType: text(row.MainLineType), UserSegment: text(row.UserSegment),
			FocusTopics: focusTopics, PainTag: text(row.PainTag), SourceUpdatedAt: ts(row.SourceUpdatedAt.UTC()), SyncedAt: ts(row.SyncedAt.UTC()),
		}); err != nil {
			return err
		}
	}
	if err := queries.DeleteMissingHXCCurrent(ctx, ids); err != nil {
		return err
	}
	if err := insertCurrentSyncRun(ctx, queries, "success", summary, now, nil); err != nil {
		return err
	}
	return queries.DeleteExpiredHXCCurrentSyncRuns(ctx, ts(now.UTC()))
}

func (repository *CurrentRepository) RecordFailure(ctx context.Context, now time.Time, code string) error {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	queries := hxcdb.New(tx)
	if err := insertCurrentSyncRun(ctx, queries, "failed", hxcport.SyncSummary{}, now, &code); err != nil {
		return err
	}
	return queries.DeleteExpiredHXCCurrentSyncRuns(ctx, ts(now.UTC()))
}

func insertCurrentSyncRun(ctx context.Context, queries *hxcdb.Queries, status string, summary hxcport.SyncSummary, now time.Time, code *string) error {
	return queries.InsertHXCCurrentSyncRun(ctx, hxcdb.InsertHXCCurrentSyncRunParams{
		Status: status, SourceCount: summary.SourceCount, MatchedCount: summary.MatchedCount,
		UnmatchedCount: summary.UnmatchedCount, ConflictCount: summary.ConflictCount,
		ErrorCode: text(code), CreatedAt: ts(now.UTC()),
	})
}

func (repository *CurrentRepository) ReadCustomerCurrent(ctx context.Context, customerID contactport.CustomerID) (hxcport.CurrentSnapshot, error) {
	if repository == nil || repository.pool == nil || ctx == nil || customerID <= 0 {
		return hxcport.CurrentSnapshot{}, errors.New("invalid hxc current read")
	}
	queries := hxcdb.New(repository.pool)
	result := hxcport.CurrentSnapshot{}
	last, err := queries.GetLastSuccessfulHXCCurrentSync(ctx)
	if err == nil && last.Valid {
		value := last.Time.UTC()
		result.LastSyncedAt = &value
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	stored, err := queries.GetHXCCurrentByCustomer(ctx, pgtype.Int8{Int64: int64(customerID), Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !stored.CustomerID.Valid || !stored.SourceUpdatedAt.Valid || !stored.SyncedAt.Valid {
		return result, errors.New("invalid stored hxc current")
	}
	current := hxcport.Current{SourceCurrent: hxcport.SourceCurrent{
		HXCUserID: stored.HxcUserID, SubscriptionTier: stored.SubscriptionTier,
		SubscriptionExpiresAt: storedTime(stored.SubscriptionExpiresAt), MonthlyChatQuota: stored.MonthlyChatQuota,
		CurrentPeriodUsed: stored.CurrentPeriodUsed, ConsultationLimit: stored.ConsultationLimit, ConsultationUsed: stored.ConsultationUsed,
		Sessions7D: stored.Sessions7d, Sessions30D: stored.Sessions30d, SessionsTotal: stored.SessionsTotal,
		UserMessages7D: stored.UserMessages7d, UserMessages30D: stored.UserMessages30d, UserMessagesTotal: stored.UserMessagesTotal,
		LastUsedAt: storedTime(stored.LastUsedAt), LastCapability: textv(stored.LastCapability), BusinessStage: textv(stored.BusinessStage),
		MainLineType: textv(stored.MainLineType), UserSegment: textv(stored.UserSegment), PainTag: textv(stored.PainTag),
		SourceUpdatedAt: stored.SourceUpdatedAt.Time.UTC(),
	}, CustomerID: contactport.CustomerID(stored.CustomerID.Int64), MatchState: hxcport.MatchState(stored.MatchState), SyncedAt: stored.SyncedAt.Time.UTC()}
	if json.Unmarshal(stored.CapabilityUsage, &current.CapabilityUsage) != nil || json.Unmarshal(stored.FocusTopics, &current.FocusTopics) != nil {
		return result, errors.New("invalid stored hxc current json")
	}
	result.Found, result.Current = true, current
	return result, nil
}

func (repository *CurrentRepository) ReadDashboard(ctx context.Context, limit int32) (hxcport.DashboardSnapshot, error) {
	if repository == nil || repository.pool == nil || ctx == nil || limit < 1 || limit > 200 {
		return hxcport.DashboardSnapshot{}, errors.New("invalid hxc dashboard read")
	}
	result := hxcport.DashboardSnapshot{Rows: []hxcport.DashboardRow{}}
	if err := repository.pool.QueryRow(ctx, `SELECT
COUNT(*)::bigint,
COUNT(*) FILTER (WHERE match_state='matched')::bigint,
COUNT(*) FILTER (WHERE match_state='unmatched')::bigint,
COUNT(*) FILTER (WHERE match_state='conflict')::bigint,
(SELECT created_at FROM hxc_current_sync_runs WHERE status='success' ORDER BY created_at DESC LIMIT 1)
FROM hxc_user_current`).Scan(&result.Total, &result.MatchedCount, &result.UnmatchedCount, &result.ConflictCount, &result.LastSyncedAt); err != nil {
		return result, err
	}
	rows, err := repository.pool.Query(ctx, `SELECT hxc_user_id, match_state, subscription_tier,
current_period_used, monthly_chat_quota, user_messages_7d, user_messages_30d,
last_used_at, last_capability, business_stage, user_segment, source_updated_at, synced_at
FROM hxc_user_current
ORDER BY COALESCE(last_used_at, source_updated_at) DESC, hxc_user_id
LIMIT $1`, limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var row hxcport.DashboardRow
		if err = rows.Scan(&row.HXCUserID, &row.MatchState, &row.SubscriptionTier,
			&row.CurrentPeriodUsed, &row.MonthlyChatQuota, &row.UserMessages7D, &row.UserMessages30D,
			&row.LastUsedAt, &row.LastCapability, &row.BusinessStage, &row.UserSegment, &row.SourceUpdatedAt, &row.SyncedAt); err != nil {
			return result, err
		}
		row.SourceUpdatedAt = row.SourceUpdatedAt.UTC()
		row.SyncedAt = row.SyncedAt.UTC()
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

func storedTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

var _ hxcport.CurrentReader = (*CurrentRepository)(nil)
var _ hxcport.DashboardReader = (*CurrentRepository)(nil)
