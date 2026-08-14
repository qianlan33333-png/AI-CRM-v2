// Package store owns Stats projections and idempotent event receipts. SQL and
// transaction-bound persistence stay here; the consumer only uses Events ports.
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	statsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/stats/store/generated"
)

const MetricCustomerTagApplied = eventport.EvTagApplied

var (
	ErrInvalidTagAppliedProjection = errors.New("invalid stats tag-applied projection")
	ErrTagAppliedReceiptConflict   = errors.New("stats tag-applied receipt conflict")
)

type TagAppliedProjection struct {
	StatDate time.Time
	TagID    int64
	Value    int64
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) GetTagApplied(ctx context.Context, statDate time.Time, tagID int64) (TagAppliedProjection, error) {
	if repository == nil || repository.pool == nil || statDate.IsZero() || tagID <= 0 {
		return TagAppliedProjection{}, ErrInvalidTagAppliedProjection
	}
	dims, err := tagDimensions(tagID)
	if err != nil {
		return TagAppliedProjection{}, err
	}
	row, err := statsdb.New(repository.pool).GetStatsDaily(ctx, statsdb.GetStatsDailyParams{
		StatDate: pgtype.Date{Time: dayUTC(statDate), Valid: true}, MetricKey: MetricCustomerTagApplied, Dims: dims,
	})
	if err != nil {
		return TagAppliedProjection{}, err
	}
	value, err := row.Value.Int64Value()
	if err != nil || !value.Valid || value.Int64 < 0 {
		return TagAppliedProjection{}, errors.Join(ErrInvalidTagAppliedProjection, err)
	}
	return TagAppliedProjection{StatDate: dayUTC(row.StatDate.Time), TagID: tagID, Value: value.Int64}, nil
}

type tagAppliedConsumer struct {
	uow        platformport.UnitOfWork
	repository *Repository
	deliveries eventport.DeliveryCompleter
}

var _ eventport.DeliverySubscriber = (*tagAppliedConsumer)(nil)

func NewTagAppliedConsumer(uow platformport.UnitOfWork, repository *Repository, deliveries eventport.DeliveryCompleter) (eventport.DeliverySubscriber, error) {
	if uow == nil || repository == nil || repository.pool == nil || deliveries == nil {
		return nil, ErrInvalidTagAppliedProjection
	}
	return &tagAppliedConsumer{uow: uow, repository: repository, deliveries: deliveries}, nil
}

func (*tagAppliedConsumer) Consumer() string     { return eventport.ConsumerStatsTagApplied }
func (*tagAppliedConsumer) EventTypes() []string { return []string{eventport.EvTagApplied} }

func (consumer *tagAppliedConsumer) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	payload, err := decodeTagApplied(claim)
	if err != nil {
		return eventport.PoisonDelivery(err)
	}
	statDate := dayUTC(claim.Record.OccurredAt)
	dims, err := tagDimensions(payload.TagID)
	if err != nil {
		return eventport.PoisonDelivery(err)
	}
	return consumer.uow.Within(ctx, func(txCtx context.Context) error {
		queries, queryErr := statsQueries(txCtx)
		if queryErr != nil {
			return queryErr
		}
		params := statsdb.ReserveStatsEventReceiptParams{
			EventID: int64(claim.Record.ID), Consumer: claim.Consumer,
			StatDate: pgtype.Date{Time: statDate, Valid: true}, MetricKey: MetricCustomerTagApplied,
			Dims: dims, ValueDelta: 1,
		}
		_, queryErr = queries.ReserveStatsEventReceipt(txCtx, params)
		if errors.Is(queryErr, pgx.ErrNoRows) {
			_, queryErr = queries.GetMatchingStatsEventReceipt(txCtx, statsdb.GetMatchingStatsEventReceiptParams(params))
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return eventport.PoisonDelivery(ErrTagAppliedReceiptConflict)
			}
			if queryErr != nil {
				return queryErr
			}
			return consumer.deliveries.Complete(txCtx, claim.Record.ID, claim.Consumer, claim.Owner)
		}
		if queryErr != nil {
			return queryErr
		}
		if queryErr = queries.IncrementStatsDaily(txCtx, statsdb.IncrementStatsDailyParams{
			StatDate: params.StatDate, MetricKey: params.MetricKey, Dims: params.Dims, ValueDelta: params.ValueDelta,
		}); queryErr != nil {
			return queryErr
		}
		return consumer.deliveries.Complete(txCtx, claim.Record.ID, claim.Consumer, claim.Owner)
	})
}

type tagAppliedPayload struct {
	CustomerID int64  `json:"customer_id"`
	TagID      int64  `json:"tag_id"`
	Actor      string `json:"actor"`
}

func decodeTagApplied(claim eventport.DeliveryClaim) (tagAppliedPayload, error) {
	if claim.Record.ID <= 0 || claim.Record.Type != eventport.EvTagApplied || claim.Record.CustomerID <= 0 ||
		claim.Consumer != eventport.ConsumerStatsTagApplied || claim.Owner == "" || claim.Status != eventport.DeliveryProcessing || claim.Record.OccurredAt.IsZero() {
		return tagAppliedPayload{}, ErrInvalidTagAppliedProjection
	}
	decoder := json.NewDecoder(bytes.NewReader(claim.Record.Payload))
	decoder.DisallowUnknownFields()
	var payload tagAppliedPayload
	if err := decoder.Decode(&payload); err != nil {
		return payload, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return payload, ErrInvalidTagAppliedProjection
	}
	if payload.CustomerID != int64(claim.Record.CustomerID) || payload.TagID <= 0 || payload.Actor == "" ||
		len(payload.Actor) > 200 || strings.TrimSpace(payload.Actor) != payload.Actor {
		return payload, ErrInvalidTagAppliedProjection
	}
	return payload, nil
}

func statsQueries(ctx context.Context) (*statsdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return statsdb.New(tx), nil
}

func tagDimensions(tagID int64) ([]byte, error) {
	if tagID <= 0 {
		return nil, ErrInvalidTagAppliedProjection
	}
	return json.Marshal(struct {
		TagID int64 `json:"tag_id"`
	}{TagID: tagID})
}

func dayUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
