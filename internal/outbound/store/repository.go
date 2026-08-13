// Package store owns transaction-bound persistence of outbound tasks.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

var _ outboundapp.TaskRepository = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) CreateAcceptedTask(ctx context.Context, command outboundapp.OneCommand) (outboundapp.TaskID, error) {
	if repository == nil || command.CustomerID <= 0 || command.TemplateKey != outboundapp.TemplateTextNoticeV1 {
		return 0, outboundapp.ErrInvalidOneCommand
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	id, err := outbounddb.New(tx).CreateAcceptedOutboundTask(ctx, outbounddb.CreateAcceptedOutboundTaskParams{
		CustomerID:  command.CustomerID,
		TemplateKey: command.TemplateKey,
		Payload:     command.Payload,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, outboundapp.ErrInvalidOneCommand
	}
	if err != nil || id <= 0 {
		return 0, errors.Join(outboundapp.ErrAcceptOneFailed, err)
	}
	return outboundapp.TaskID(id), nil
}
