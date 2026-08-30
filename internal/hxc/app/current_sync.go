package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrHXCCurrentSync = errors.New("hxc current sync failed")

const maxSourceRows = 10_000

var allowedCapabilities = map[string]struct{}{
	"peer_chat": {}, "coach_consult": {}, "lesson": {}, "assessment": {}, "weekly_review": {},
}

type CurrentSource interface {
	ReadCurrent(context.Context) ([]hxcport.SourceCurrent, error)
}

type IdentityResolver interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
}

type CurrentStore interface {
	Replace(context.Context, []hxcport.Current, hxcport.SyncSummary, time.Time) error
	RecordFailure(context.Context, time.Time, string) error
}

type CurrentSyncService struct {
	source       CurrentSource
	identities   IdentityResolver
	store        CurrentStore
	uow          platformport.UnitOfWork
	unionIDScope string
	clock        func() time.Time
}

func NewCurrentSyncService(source CurrentSource, identities IdentityResolver, store CurrentStore, uow platformport.UnitOfWork, unionIDScope string, clock func() time.Time) (*CurrentSyncService, error) {
	if source == nil || identities == nil || store == nil || uow == nil || clock == nil || !strings.HasPrefix(unionIDScope, "wechat-open-platform:") {
		return nil, ErrHXCCurrentSync
	}
	return &CurrentSyncService{source: source, identities: identities, store: store, uow: uow, unionIDScope: unionIDScope, clock: clock}, nil
}

func (service *CurrentSyncService) Sync(ctx context.Context) (hxcport.SyncSummary, error) {
	if service == nil || ctx == nil {
		return hxcport.SyncSummary{}, ErrHXCCurrentSync
	}
	now := service.clock().UTC()
	if now.IsZero() {
		return hxcport.SyncSummary{}, ErrHXCCurrentSync
	}
	sourceRows, err := service.source.ReadCurrent(ctx)
	if err != nil || len(sourceRows) == 0 || len(sourceRows) > maxSourceRows {
		service.recordFailure(ctx, now, "source_unavailable")
		return hxcport.SyncSummary{}, errors.Join(ErrHXCCurrentSync, err)
	}
	rows := make([]hxcport.Current, len(sourceRows))
	seen := make(map[string]struct{}, len(sourceRows))
	for index, source := range sourceRows {
		if err := validateSourceCurrent(source); err != nil {
			service.recordFailure(ctx, now, "source_invalid")
			return hxcport.SyncSummary{}, errors.Join(ErrHXCCurrentSync, err)
		}
		if _, duplicate := seen[source.HXCUserID]; duplicate {
			service.recordFailure(ctx, now, "source_duplicate")
			return hxcport.SyncSummary{}, ErrHXCCurrentSync
		}
		seen[source.HXCUserID] = struct{}{}
		customerID, state, err := service.match(ctx, source)
		if err != nil {
			service.recordFailure(ctx, now, "identity_unavailable")
			return hxcport.SyncSummary{}, errors.Join(ErrHXCCurrentSync, err)
		}
		rows[index] = hxcport.Current{SourceCurrent: source, CustomerID: customerID, MatchState: state, SyncedAt: now}
	}
	downgradeDuplicateCustomers(rows)
	summary := summarize(rows)
	if err := service.uow.Within(ctx, func(txCtx context.Context) error { return service.store.Replace(txCtx, rows, summary, now) }); err != nil {
		service.recordFailure(ctx, now, "target_write_failed")
		return hxcport.SyncSummary{}, errors.Join(ErrHXCCurrentSync, err)
	}
	return summary, nil
}

func (service *CurrentSyncService) recordFailure(ctx context.Context, now time.Time, code string) {
	_ = service.uow.Within(context.WithoutCancel(ctx), func(txCtx context.Context) error {
		return service.store.RecordFailure(txCtx, now, code)
	})
}

func (service *CurrentSyncService) match(ctx context.Context, source hxcport.SourceCurrent) (contactport.CustomerID, hxcport.MatchState, error) {
	results := make([]identityport.ResolveResult, 0, 2)
	if source.UnionID != "" {
		result, err := service.identities.Resolve(ctx, identityport.IDRef{Kind: identityport.KindUnionID, Scope: service.unionIDScope, Value: source.UnionID})
		if err != nil {
			return 0, hxcport.MatchStateConflict, err
		}
		results = append(results, result)
	}
	if source.Phone != "" {
		result, err := service.identities.Resolve(ctx, identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: source.Phone})
		if err != nil {
			return 0, hxcport.MatchStateConflict, err
		}
		results = append(results, result)
	}
	var found contactport.CustomerID
	for _, result := range results {
		if result.Status == identityport.ResolveConflict || (result.Status == identityport.ResolveFound && found != 0 && found != result.CustomerID) {
			return 0, hxcport.MatchStateConflict, nil
		}
		if result.Status == identityport.ResolveFound {
			found = result.CustomerID
		}
	}
	if found != 0 {
		return found, hxcport.MatchStateMatched, nil
	}
	return 0, hxcport.MatchStateUnmatched, nil
}

func validateSourceCurrent(row hxcport.SourceCurrent) error {
	if row.HXCUserID == "" || strings.TrimSpace(row.HXCUserID) != row.HXCUserID || utf8.RuneCountInString(row.HXCUserID) > 200 ||
		row.SubscriptionTier == "" || strings.TrimSpace(row.SubscriptionTier) != row.SubscriptionTier || utf8.RuneCountInString(row.SubscriptionTier) > 100 || row.SourceUpdatedAt.IsZero() ||
		row.CapabilityUsage == nil || row.FocusTopics == nil ||
		row.MonthlyChatQuota < 0 || row.CurrentPeriodUsed < 0 || row.ConsultationLimit < 0 || row.ConsultationUsed < 0 ||
		row.Sessions7D < 0 || row.Sessions30D < row.Sessions7D || row.SessionsTotal < row.Sessions30D ||
		row.UserMessages7D < 0 || row.UserMessages30D < row.UserMessages7D || row.UserMessagesTotal < row.UserMessages30D {
		return ErrHXCCurrentSync
	}
	if len(row.FocusTopics) > 100 || invalidOptionalCurrentText(row.BusinessStage) || invalidOptionalCurrentText(row.MainLineType) || invalidOptionalCurrentText(row.UserSegment) || invalidOptionalCurrentText(row.PainTag) || invalidOptionalCurrentText(row.LastCapability) ||
		(row.LastCapability != nil && !allowedCapability(*row.LastCapability)) || invalidOptionalCurrentTime(row.SubscriptionExpiresAt) || invalidOptionalCurrentTime(row.LastUsedAt) {
		return ErrHXCCurrentSync
	}
	for key, usage := range row.CapabilityUsage {
		if _, ok := allowedCapabilities[key]; !ok || usage.Count7D < 0 || usage.Count30D < usage.Count7D || usage.CountTotal < usage.Count30D {
			return ErrHXCCurrentSync
		}
	}
	if _, err := json.Marshal(row.CapabilityUsage); err != nil {
		return fmt.Errorf("capability usage: %w", err)
	}
	seenTopics := make(map[string]struct{}, len(row.FocusTopics))
	for _, topic := range row.FocusTopics {
		if topic == "" || strings.TrimSpace(topic) != topic || utf8.RuneCountInString(topic) > 100 {
			return ErrHXCCurrentSync
		}
		if _, duplicate := seenTopics[topic]; duplicate {
			return ErrHXCCurrentSync
		}
		seenTopics[topic] = struct{}{}
	}
	return nil
}

func allowedCapability(value string) bool { _, ok := allowedCapabilities[value]; return ok }

func invalidOptionalCurrentText(value *string) bool {
	return value != nil && (*value == "" || strings.TrimSpace(*value) != *value || utf8.RuneCountInString(*value) > 100)
}

func invalidOptionalCurrentTime(value *time.Time) bool { return value != nil && value.IsZero() }

func downgradeDuplicateCustomers(rows []hxcport.Current) {
	counts := make(map[contactport.CustomerID]int)
	for _, row := range rows {
		if row.MatchState == hxcport.MatchStateMatched {
			counts[row.CustomerID]++
		}
	}
	for index := range rows {
		if counts[rows[index].CustomerID] > 1 {
			rows[index].CustomerID = 0
			rows[index].MatchState = hxcport.MatchStateConflict
		}
	}
}

func summarize(rows []hxcport.Current) hxcport.SyncSummary {
	result := hxcport.SyncSummary{SourceCount: int32(len(rows))}
	for _, row := range rows {
		switch row.MatchState {
		case hxcport.MatchStateMatched:
			result.MatchedCount++
		case hxcport.MatchStateUnmatched:
			result.UnmatchedCount++
		case hxcport.MatchStateConflict:
			result.ConflictCount++
		}
	}
	return result
}
