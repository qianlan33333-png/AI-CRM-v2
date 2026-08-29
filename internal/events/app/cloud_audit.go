package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

var (
	ErrCloudAuditInvalid     = errors.New("invalid cloud audit query")
	ErrCloudAuditUnavailable = errors.New("cloud audit unavailable")
)

type CloudAuditResult struct {
	Filter                   eventport.CloudAuditFilter `json:"filter"`
	Items                    []eventport.CloudAuditFact `json:"items"`
	ObservedAt               time.Time                  `json:"observed_at"`
	LocalFactsOnly           bool                       `json:"local_facts_only"`
	RealExternalCallExecuted bool                       `json:"real_external_call_executed"`
	DeliveryProven           bool                       `json:"delivery_proven"`
}

type CloudAuditService struct {
	repository eventport.CloudAuditRepository
	now        func() time.Time
}

func NewCloudAuditService(repository eventport.CloudAuditRepository, now func() time.Time) *CloudAuditService {
	if now == nil {
		now = time.Now
	}
	return &CloudAuditService{repository: repository, now: now}
}

func (service *CloudAuditService) List(ctx context.Context, filter eventport.CloudAuditFilter) (CloudAuditResult, error) {
	filter.TraceID, filter.SessionID = strings.TrimSpace(filter.TraceID), strings.TrimSpace(filter.SessionID)
	if !validCloudAuditFilter(filter) {
		return CloudAuditResult{}, ErrCloudAuditInvalid
	}
	if service == nil || service.repository == nil || ctx == nil || ctx.Err() != nil {
		return CloudAuditResult{}, ErrCloudAuditUnavailable
	}
	items, err := service.repository.ListCloudAudit(ctx, filter)
	if err != nil {
		return CloudAuditResult{}, errors.Join(ErrCloudAuditUnavailable, err)
	}
	for index, item := range items {
		if item.EventID < 1 || !validCloudAuditText(item.EventType) || item.OccurredAt.IsZero() ||
			item.Pending < 0 || item.Processing < 0 || item.Completed < 0 || item.FinalFailed < 0 || item.OutcomeUnknown < 0 ||
			index > 0 && (items[index-1].OccurredAt.Before(item.OccurredAt) || items[index-1].OccurredAt.Equal(item.OccurredAt) && items[index-1].EventID <= item.EventID) {
			return CloudAuditResult{}, ErrCloudAuditUnavailable
		}
	}
	observedAt := service.now().UTC()
	if observedAt.IsZero() {
		return CloudAuditResult{}, ErrCloudAuditUnavailable
	}
	return CloudAuditResult{Filter: filter, Items: append([]eventport.CloudAuditFact(nil), items...), ObservedAt: observedAt, LocalFactsOnly: true}, nil
}

func validCloudAuditFilter(filter eventport.CloudAuditFilter) bool {
	return (filter.TraceID != "" || filter.SessionID != "") && validCloudAuditOptionalText(filter.TraceID) &&
		validCloudAuditOptionalText(filter.SessionID) && filter.Limit >= 1 && filter.Limit <= 100
}

func validCloudAuditOptionalText(value string) bool {
	return value == "" || validCloudAuditText(value)
}

func validCloudAuditText(value string) bool {
	if value == "" || len(value) > 200 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
