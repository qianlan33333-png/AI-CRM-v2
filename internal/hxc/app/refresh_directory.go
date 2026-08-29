package app

import (
	"context"
	"errors"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

var (
	ErrDirectoryRefreshInvalid     = errors.New("invalid hxc directory refresh command")
	ErrDirectoryRefreshUnavailable = errors.New("hxc directory refresh unavailable")
)

type OperationMemberRefresher interface {
	RefreshOperationMembers(context.Context, groupopsport.OperationMemberRefreshCommand) (groupopsport.OperationMemberPage, error)
}

type SenderProjectionReader interface {
	Read(context.Context) (Projection, error)
}

type RefreshDirectoryCommand struct {
	ActorID        int64
	IdempotencyKey string
	PageSize       int32
}

type RefreshDirectoryResult struct {
	SyncedCount          int        `json:"synced_count"`
	ProviderReadExecuted bool       `json:"provider_read_executed"`
	Projection           Projection `json:"projection"`
}

// DirectoryRefresher reuses the canonical operation-members Provider read and
// only returns after the local staff projection can be read back.
type DirectoryRefresher struct {
	members    OperationMemberRefresher
	projection SenderProjectionReader
}

func NewDirectoryRefresher(members OperationMemberRefresher, projection SenderProjectionReader) *DirectoryRefresher {
	return &DirectoryRefresher{members: members, projection: projection}
}

func (service *DirectoryRefresher) Refresh(ctx context.Context, command RefreshDirectoryCommand) (RefreshDirectoryResult, error) {
	if ctx == nil || service == nil || service.members == nil || service.projection == nil || command.ActorID < 1 || command.PageSize < 1 || command.PageSize > 100 || len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 128 {
		return RefreshDirectoryResult{}, ErrDirectoryRefreshInvalid
	}
	page, err := service.members.RefreshOperationMembers(ctx, groupopsport.OperationMemberRefreshCommand{
		ActorID: command.ActorID, PageSize: command.PageSize, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil || page.Scope != "group_ops" || page.PageSize != command.PageSize || len(page.Items) > int(command.PageSize) {
		return RefreshDirectoryResult{}, errors.Join(ErrDirectoryRefreshUnavailable, err)
	}
	projection, err := service.projection.Read(ctx)
	if err != nil {
		return RefreshDirectoryResult{}, errors.Join(ErrDirectoryRefreshUnavailable, err)
	}
	return RefreshDirectoryResult{SyncedCount: len(page.Items), ProviderReadExecuted: true, Projection: projection}, nil
}
