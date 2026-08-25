package main

import (
	"context"

	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

type groupOpsOperationMemberApplication struct {
	legacyaudience.LocalConfigurationApplication
	runtime *groupopsapp.RuntimeService
}

var _ legacyaudience.GroupOpsOperationMemberApplication = groupOpsOperationMemberApplication{}

func (application groupOpsOperationMemberApplication) ListGroupOpsOperationMembers(ctx context.Context, pageSize int) (any, error) {
	return application.runtime.ListOperationMembers(ctx, int32(pageSize))
}

func (application groupOpsOperationMemberApplication) RefreshGroupOpsOperationMembers(ctx context.Context, actor int64, key string, pageSize int) (any, error) {
	return application.runtime.RefreshOperationMembers(ctx, groupopsport.OperationMemberRefreshCommand{ActorID: actor, PageSize: int32(pageSize), IdempotencyKey: key})
}
