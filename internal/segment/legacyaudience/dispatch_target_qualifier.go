package legacyaudience

import (
	"context"
	"sort"
	"strings"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

// AudienceDispatchTargetQualifier preserves the old Audience invariant: a
// target may be sent only by its current active relationship owner, and only
// when that exact WeCom userid is enabled in the package sender whitelist.
// It never chooses the next whitelist entry as a substitute sender.
type AudienceDispatchTargetQualifier struct {
	repository *SQLRepository
	targets    contactport.WeComOutboundTargetResolver
}

var _ outboundport.AudienceDispatchTargetQualifier = (*AudienceDispatchTargetQualifier)(nil)

func NewAudienceDispatchTargetQualifier(repository *SQLRepository, targets contactport.WeComOutboundTargetResolver) (*AudienceDispatchTargetQualifier, error) {
	if repository == nil || targets == nil {
		return nil, ErrUnavailable
	}
	return &AudienceDispatchTargetQualifier{repository: repository, targets: targets}, nil
}

func (qualifier *AudienceDispatchTargetQualifier) QualifyAudienceDispatchTargets(ctx context.Context, packageID int64, customerIDs []int64) ([]outboundport.AudienceDispatchTargetQualification, error) {
	if qualifier == nil || qualifier.repository == nil || qualifier.targets == nil || ctx == nil || ctx.Err() != nil || packageID < 1 || len(customerIDs) == 0 {
		return nil, ErrInvalidInput
	}
	ids := append([]int64(nil), customerIDs...)
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for index, id := range ids {
		if id < 1 || (index > 0 && id == ids[index-1]) {
			return nil, ErrInvalidInput
		}
	}
	database, err := qualifier.repository.transaction(ctx)
	if err != nil {
		return nil, err
	}
	senders, err := listPackageSenders(ctx, database, packageID, true)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(senders))
	for _, sender := range senders {
		if sender.IsEnabled {
			allowed[sender.SenderUserID] = struct{}{}
		}
	}
	result := make([]outboundport.AudienceDispatchTargetQualification, 0, len(ids))
	for _, customerID := range ids {
		sender, external, resolved, resolveErr := qualifier.targets.Resolve(ctx, customerID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value := outboundport.AudienceDispatchTargetQualification{CustomerID: customerID}
		if !resolved || !validAudienceDispatchTarget(sender, 128) || !validAudienceDispatchTarget(external, 1024) {
			value.Exclusion = "target_unresolved"
		} else if _, present := allowed[sender]; !present {
			value.Exclusion = "sender_not_allowed"
		} else {
			value.Eligible, value.SenderUserID, value.ExternalUserID = true, sender, external
		}
		result = append(result, value)
	}
	return result, nil
}

func validAudienceDispatchTarget(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value)
}
