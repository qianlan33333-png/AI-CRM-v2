package membergrid

import (
	"context"
	"errors"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type PublicShareBucket struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}

type PublicShareSummary struct {
	Buckets []PublicShareBucket `json:"buckets"`
	AsOf    time.Time           `json:"as_of"`
}

type PublicShareSummaryStore interface {
	SummarizePublicMembers(context.Context, int64) ([]PublicShareBucket, error)
}

type PublicShareService struct {
	uow     platformport.UnitOfWork
	shares  ExternalShareStore
	summary PublicShareSummaryStore
	tokens  *ExternalShareTokenCodec
	now     func() time.Time
}

func NewPublicShareService(uow platformport.UnitOfWork, shares ExternalShareStore, summary PublicShareSummaryStore, tokens *ExternalShareTokenCodec) (*PublicShareService, error) {
	if nilDependency(uow) || nilDependency(shares) || nilDependency(summary) || tokens == nil {
		return nil, errors.New("member grid public share dependencies are required")
	}
	return &PublicShareService{uow: uow, shares: shares, summary: summary, tokens: tokens, now: time.Now}, nil
}

func (service *PublicShareService) Summary(ctx context.Context, token string) (PublicShareSummary, error) {
	if service == nil || nilDependency(service.uow) || nilDependency(service.shares) || nilDependency(service.summary) || service.tokens == nil || service.now == nil || ctx == nil {
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
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		share, lookupErr := service.shares.LookupEnabledExternalShare(txCtx, shareID)
		if lookupErr != nil {
			return lookupErr
		}
		if !validExternalShare(share) || !share.Enabled || share.ShareID != shareID {
			return ErrNotFound
		}
		buckets, lookupErr = service.summary.SummarizePublicMembers(txCtx, share.ServiceProductID)
		return lookupErr
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidExternalShareToken) {
			return PublicShareSummary{}, ErrNotFound
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
	return PublicShareSummary{Buckets: closed, AsOf: asOf}, nil
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
