package app

import (
	"context"
	"errors"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrAudienceEditableProjection = errors.New("audience editable projection failed")

var deferredRedesignAudiencePackageKeys = []string{
	"huangyoucan_wecom_unregistered",
	"audience_huangxiaocan_active_not_ai_opc_not_paid",
	"audience_hxc_member_registered_unused",
}

func DeferredRedesignAudiencePackageKeys() []string {
	return append([]string(nil), deferredRedesignAudiencePackageKeys...)
}

type AudienceEditableProjectionResult struct {
	GroupsCreated        int `json:"groups_created"`
	PackagesCreated      int `json:"packages_created"`
	MembersProjected     int `json:"members_projected"`
	GroupsReplayed       int `json:"groups_replayed"`
	PackagesReplayed     int `json:"packages_replayed"`
	HistoryOnlyPreserved int `json:"history_only_preserved"`
}

type AudienceEditableProjectionStore interface {
	ProjectActiveAudienceHistory(context.Context, int64, time.Time) (AudienceEditableProjectionResult, error)
}

type AudienceEditableProjectionService struct {
	uow   platformport.UnitOfWork
	store AudienceEditableProjectionStore
}

func NewAudienceEditableProjectionService(uow platformport.UnitOfWork, store AudienceEditableProjectionStore) *AudienceEditableProjectionService {
	return &AudienceEditableProjectionService{uow: uow, store: store}
}

func (service *AudienceEditableProjectionService) Project(ctx context.Context, actorID int64, at time.Time) (AudienceEditableProjectionResult, error) {
	if service == nil || service.uow == nil || service.store == nil || ctx == nil || actorID < 1 || at.IsZero() || at.Location() != time.UTC {
		return AudienceEditableProjectionResult{}, ErrAudienceEditableProjection
	}
	var result AudienceEditableProjectionResult
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = service.store.ProjectActiveAudienceHistory(tx, actorID, at)
		return err
	})
	if err != nil {
		return AudienceEditableProjectionResult{}, errors.Join(ErrAudienceEditableProjection, err)
	}
	return result, nil
}
