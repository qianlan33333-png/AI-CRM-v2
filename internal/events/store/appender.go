package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Appender struct{}

var _ eventport.Appender = (*Appender)(nil)

func NewAppender() *Appender {
	return &Appender{}
}

func (a *Appender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	if err := validate(event); err != nil {
		return 0, err
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}

	params := eventdb.AppendEventParams{
		EventType:      event.Type,
		Payload:        event.Payload,
		OccurredAt:     pgtype.Timestamptz{Time: event.OccurredAt, Valid: true},
		IdempotencyKey: event.IdempotencyKey,
	}
	if event.CustomerID > 0 {
		params.CustomerID = pgtype.Int8{Int64: int64(event.CustomerID), Valid: true}
	}
	id, err := eventdb.New(tx).AppendEvent(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, eventport.ErrIdempotencyConflict
	}
	if err != nil {
		return 0, err
	}
	return eventport.EventID(id), nil
}

func validate(event eventport.Event) error {
	if strings.TrimSpace(event.Type) == "" || event.CustomerID < 0 ||
		event.OccurredAt.IsZero() || strings.TrimSpace(event.IdempotencyKey) == "" ||
		len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return eventport.ErrInvalidEvent
	}
	return nil
}
