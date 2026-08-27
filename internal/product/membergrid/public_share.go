package membergrid

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type PublicShareBucket struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}

type PublicShareSummary struct {
	Buckets    []PublicShareBucket `json:"buckets"`
	Rows       []PublicShareMember `json:"rows"`
	Limit      int                 `json:"limit"`
	NextCursor string              `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
	AsOf       time.Time           `json:"as_of"`
}

type PublicShareMember struct {
	DisplayName string     `json:"display_name"`
	State       string     `json:"state"`
	Source      string     `json:"source"`
	StartsAt    time.Time  `json:"starts_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PublicShareSummaryStore interface {
	SummarizePublicMembers(context.Context, int64) ([]PublicShareBucket, error)
}

type PublicShareMemberRecord struct {
	MemberRef   string
	State       StateFilter
	Source      SourceFilter
	StartsAt    time.Time
	ExpiresAt   *time.Time
	UpdatedAt   time.Time
	DisplayName string
}

type PublicShareMemberStore interface {
	QueryPublicMembers(context.Context, StoreQuery) ([]PublicShareMemberRecord, error)
}

type PublicShareService struct {
	uow     platformport.UnitOfWork
	shares  ExternalShareStore
	summary PublicShareSummaryStore
	members PublicShareMemberStore
	tokens  *ExternalShareTokenCodec
	cursors *CursorCodec
	now     func() time.Time
}

func NewPublicShareService(uow platformport.UnitOfWork, shares ExternalShareStore, summary PublicShareSummaryStore, members PublicShareMemberStore, tokens *ExternalShareTokenCodec, cursors *CursorCodec) (*PublicShareService, error) {
	if nilDependency(uow) || nilDependency(shares) || nilDependency(summary) || nilDependency(members) || tokens == nil || cursors == nil || cursors.aead == nil {
		return nil, errors.New("member grid public share dependencies are required")
	}
	return &PublicShareService{uow: uow, shares: shares, summary: summary, members: members, tokens: tokens, cursors: cursors, now: time.Now}, nil
}

func (service *PublicShareService) Summary(ctx context.Context, token, cursor string) (PublicShareSummary, error) {
	if service == nil || nilDependency(service.uow) || nilDependency(service.shares) || nilDependency(service.summary) || nilDependency(service.members) || service.tokens == nil || service.cursors == nil || service.now == nil || ctx == nil {
		return PublicShareSummary{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return PublicShareSummary{}, errors.Join(ErrUnavailable, err)
	}
	shareID, err := service.tokens.Verify(token)
	if err != nil {
		return PublicShareSummary{}, ErrNotFound
	}

	var buckets []PublicShareBucket
	var records []PublicShareMemberRecord
	var after *Position
	var serviceProductID int64
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		share, lookupErr := service.shares.LookupEnabledExternalShare(txCtx, shareID)
		if lookupErr != nil {
			return lookupErr
		}
		if !validExternalShare(share) || !share.Enabled || share.ShareID != shareID {
			return ErrNotFound
		}
		serviceProductID = share.ServiceProductID
		if cursor != "" {
			decoded, decodeErr := service.cursors.Decode(cursor, share.ServiceProductID, StateAll, SourceAny, MaximumLimit)
			if decodeErr != nil {
				return ErrInvalidCursor
			}
			after = &decoded
		}
		buckets, lookupErr = service.summary.SummarizePublicMembers(txCtx, share.ServiceProductID)
		if lookupErr != nil {
			return lookupErr
		}
		records, lookupErr = service.members.QueryPublicMembers(txCtx, StoreQuery{
			ProductID: share.ServiceProductID,
			State:     StateAll,
			Source:    SourceAny,
			Limit:     MaximumLimit + 1,
			After:     clonePosition(after),
		})
		if lookupErr != nil {
			return lookupErr
		}
		if len(records) > MaximumLimit+1 || !validPublicShareRecords(records, after) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidExternalShareToken) {
			return PublicShareSummary{}, ErrNotFound
		}
		if errors.Is(err, ErrInvalidCursor) {
			return PublicShareSummary{}, ErrInvalidCursor
		}
		return PublicShareSummary{}, errors.Join(ErrUnavailable, err)
	}
	closed, ok := closePublicShareBuckets(buckets)
	if !ok {
		return PublicShareSummary{}, ErrUnavailable
	}
	asOf := service.now().UTC()
	if asOf.IsZero() {
		return PublicShareSummary{}, ErrUnavailable
	}
	hasMore := len(records) > MaximumLimit
	visible := records
	if hasMore {
		visible = records[:MaximumLimit]
	}
	result := PublicShareSummary{
		Buckets: closed,
		Rows:    make([]PublicShareMember, len(visible)),
		Limit:   MaximumLimit,
		HasMore: hasMore,
		AsOf:    asOf,
	}
	for index, record := range visible {
		result.Rows[index] = PublicShareMember{
			DisplayName: record.DisplayName,
			State:       string(record.State),
			Source:      string(record.Source),
			StartsAt:    record.StartsAt.UTC(),
			ExpiresAt:   cloneTime(record.ExpiresAt),
			UpdatedAt:   record.UpdatedAt.UTC(),
		}
	}
	if hasMore && len(visible) > 0 {
		last := visible[len(visible)-1]
		result.NextCursor, err = service.cursors.Encode(serviceProductID, StateAll, SourceAny, MaximumLimit, Position{
			UpdatedAt: last.UpdatedAt,
			MemberRef: last.MemberRef,
		})
		if err != nil {
			return PublicShareSummary{}, errors.Join(ErrUnavailable, err)
		}
	}
	return result, nil
}

func validPublicShareRecords(records []PublicShareMemberRecord, after *Position) bool {
	seen := make(map[string]struct{}, len(records))
	var previous *PublicShareMemberRecord
	for index := range records {
		record := records[index]
		if !validMemberRef(record.MemberRef) || record.StartsAt.IsZero() || record.UpdatedAt.IsZero() ||
			!record.State.validCanonicalGridState() || record.State == StateAll || !record.Source.valid() || record.Source == SourceAny || record.DisplayName == "" || !utf8.ValidString(record.DisplayName) {
			return false
		}
		if _, duplicate := seen[record.MemberRef]; duplicate {
			return false
		}
		seen[record.MemberRef] = struct{}{}
		if record.ExpiresAt != nil && (record.ExpiresAt.IsZero() || record.ExpiresAt.Before(record.StartsAt)) {
			return false
		}
		if index == 0 && after != nil && !publicRecordBeforePosition(record, *after) {
			return false
		}
		if previous != nil && !publicRecordBefore(record, *previous) {
			return false
		}
		previous = &records[index]
	}
	return true
}

func publicRecordBefore(current, previous PublicShareMemberRecord) bool {
	return current.UpdatedAt.Before(previous.UpdatedAt) || current.UpdatedAt.Equal(previous.UpdatedAt) && current.MemberRef < previous.MemberRef
}

func publicRecordBeforePosition(record PublicShareMemberRecord, position Position) bool {
	return record.UpdatedAt.Before(position.UpdatedAt) || record.UpdatedAt.Equal(position.UpdatedAt) && record.MemberRef < position.MemberRef
}

func closePublicShareBuckets(input []PublicShareBucket) ([]PublicShareBucket, bool) {
	counts := map[string]int64{"active": 0, "expired": 0, "removed": 0}
	seen := make(map[string]struct{}, len(input))
	for _, bucket := range input {
		if _, allowed := counts[bucket.State]; !allowed || bucket.Count < 0 {
			return nil, false
		}
		if _, duplicate := seen[bucket.State]; duplicate {
			return nil, false
		}
		seen[bucket.State] = struct{}{}
		counts[bucket.State] = bucket.Count
	}
	return []PublicShareBucket{
		{State: "active", Count: counts["active"]},
		{State: "expired", Count: counts["expired"]},
		{State: "removed", Count: counts["removed"]},
	}, true
}
