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

func storedTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

var _ hxcport.CurrentReader = (*CurrentRepository)(nil)
