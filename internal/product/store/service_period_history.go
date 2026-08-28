package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type ServicePeriodHistoryStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

type ServicePeriodHistoryReader struct{ db productdb.DBTX }

var _ productport.ServicePeriodHistoryStore = (*ServicePeriodHistoryStore)(nil)
var _ productport.ServicePeriodHistoryReader = (*ServicePeriodHistoryReader)(nil)

func NewServicePeriodHistoryStore() *ServicePeriodHistoryStore {
	return &ServicePeriodHistoryStore{tx: platformstore.TxFromContext}
}

func NewServicePeriodHistoryReader(pool *pgxpool.Pool) *ServicePeriodHistoryReader {
	reader := &ServicePeriodHistoryReader{}
	if pool != nil {
		reader.db = pool
	}
	return reader
}

func (store *ServicePeriodHistoryStore) CreateServicePeriodHistoryDefinition(ctx context.Context, value productport.ServicePeriodHistoryDefinition) (productport.ServicePeriodHistoryDefinition, error) {
	if !validServicePeriodHistoryDefinitionInput(value) {
		return productport.ServicePeriodHistoryDefinition{}, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.ServicePeriodHistoryDefinition{}, err
	}
	row, err := queries.CreateServicePeriodHistoryDefinition(ctx, productdb.CreateServicePeriodHistoryDefinitionParams{
		SourceDefinitionID: value.SourceDefinitionID, ProductID: value.ProductID,
		MembershipConfigID: value.MembershipConfigID, MembershipConfigName: value.MembershipConfigName,
		DurationDays: value.DurationDays, Deleted: value.Deleted,
		CreatedAt: servicePeriodHistoryTimestamp(value.CreatedAt), UpdatedAt: servicePeriodHistoryTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return productport.ServicePeriodHistoryDefinition{}, servicePeriodHistoryWriteError(err)
	}
	return servicePeriodHistoryDefinition(row)
}

func (store *ServicePeriodHistoryStore) GetServicePeriodHistoryDefinition(ctx context.Context, id int64) (productport.ServicePeriodHistoryDefinition, error) {
	if id < 1 {
		return productport.ServicePeriodHistoryDefinition{}, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.ServicePeriodHistoryDefinition{}, err
	}
	row, err := queries.GetServicePeriodHistoryDefinition(ctx, id)
	if err != nil {
		return productport.ServicePeriodHistoryDefinition{}, servicePeriodHistoryReadError(err)
	}
	return servicePeriodHistoryDefinition(row)
}

func (store *ServicePeriodHistoryStore) CreateServicePeriodHistoryEntitlement(ctx context.Context, value productport.ServicePeriodHistoryEntitlement) (productport.ServicePeriodHistoryEntitlement, error) {
	if !validServicePeriodHistoryEntitlementInput(value) {
		return productport.ServicePeriodHistoryEntitlement{}, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	row, err := queries.CreateServicePeriodHistoryEntitlement(ctx, productdb.CreateServicePeriodHistoryEntitlementParams{
		SourceEntitlementID: value.SourceEntitlementID, DefinitionID: value.DefinitionID,
		CustomerID: servicePeriodHistoryNullableInt64(value.CustomerID), MembershipConfigID: value.MembershipConfigID,
		Status: value.Status, StartAt: servicePeriodHistoryTimestamp(value.StartAt), EndAt: servicePeriodHistoryTimestamp(value.EndAt),
		LastOrderID: servicePeriodHistoryNullableInt64(value.LastOrderID), LastOutTradeNo: value.LastOutTradeNo,
		RenewalCount: value.RenewalCount, CreatedAt: servicePeriodHistoryTimestamp(value.CreatedAt), UpdatedAt: servicePeriodHistoryTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, servicePeriodHistoryWriteError(err)
	}
	return servicePeriodHistoryEntitlement(row)
}

func (store *ServicePeriodHistoryStore) GetServicePeriodHistoryEntitlement(ctx context.Context, id int64) (productport.ServicePeriodHistoryEntitlement, error) {
	if id < 1 {
		return productport.ServicePeriodHistoryEntitlement{}, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	row, err := queries.GetServicePeriodHistoryEntitlement(ctx, id)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, servicePeriodHistoryReadError(err)
	}
	return servicePeriodHistoryEntitlement(row)
}

func (store *ServicePeriodHistoryStore) CreateServicePeriodHistoryEvent(ctx context.Context, value productport.ServicePeriodHistoryEvent) (productport.ServicePeriodHistoryEvent, error) {
	if !validServicePeriodHistoryEventInput(value) {
		return productport.ServicePeriodHistoryEvent{}, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	row, err := queries.CreateServicePeriodHistoryEvent(ctx, productdb.CreateServicePeriodHistoryEventParams{
		SourceEventID: value.SourceEventID, DefinitionID: value.DefinitionID,
		EntitlementID: servicePeriodHistoryNullableInt64(value.EntitlementID), CustomerID: servicePeriodHistoryNullableInt64(value.CustomerID), OrderID: servicePeriodHistoryNullableInt64(value.OrderID),
		EventID: value.EventID, EventType: value.EventType, DurationDays: value.DurationDays, OutTradeNo: value.OutTradeNo,
		BeforeStartAt: servicePeriodHistoryNullableTimestamp(value.BeforeStartAt), BeforeEndAt: servicePeriodHistoryNullableTimestamp(value.BeforeEndAt),
		AfterStartAt: servicePeriodHistoryNullableTimestamp(value.AfterStartAt), AfterEndAt: servicePeriodHistoryNullableTimestamp(value.AfterEndAt),
		CreatedAt: servicePeriodHistoryTimestamp(value.CreatedAt),
	})
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, servicePeriodHistoryWriteError(err)
	}
	return servicePeriodHistoryEvent(row)
}

func (store *ServicePeriodHistoryStore) GetServicePeriodHistoryEvent(ctx context.Context, id int64) (productport.ServicePeriodHistoryEvent, error) {
	if id < 1 {
		return productport.ServicePeriodHistoryEvent{}, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	row, err := queries.GetServicePeriodHistoryEvent(ctx, id)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, servicePeriodHistoryReadError(err)
	}
	return servicePeriodHistoryEvent(row)
}

func (reader *ServicePeriodHistoryReader) ListServicePeriodHistoryDefinitions(ctx context.Context, limit, offset int32) ([]productport.ServicePeriodHistoryProduct, int64, error) {
	if !validServicePeriodHistoryPage(limit, offset) {
		return nil, 0, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountServicePeriodHistoryDefinitions(ctx)
	if err != nil || total < 0 {
		return nil, 0, productport.ErrServicePeriodHistoryUnavailable
	}
	rows, err := queries.ListServicePeriodHistoryDefinitions(ctx, productdb.ListServicePeriodHistoryDefinitionsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, productport.ErrServicePeriodHistoryUnavailable
	}
	items := make([]productport.ServicePeriodHistoryProduct, 0, len(rows))
	for _, row := range rows {
		definition, err := servicePeriodHistoryDefinition(productdb.ProductServicePeriodHistory{
			ID: row.ID, SourceDefinitionID: row.SourceDefinitionID, ProductID: row.ProductID, MembershipConfigID: row.MembershipConfigID,
			MembershipConfigName: row.MembershipConfigName, DurationDays: row.DurationDays, Deleted: row.Deleted, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
		if err != nil || row.ProductCode == "" || row.ProductName == "" || row.PriceMinor < 0 || row.Currency != "CNY" {
			return nil, 0, productport.ErrServicePeriodHistoryUnavailable
		}
		items = append(items, productport.ServicePeriodHistoryProduct{ServicePeriodHistoryDefinition: definition, ProductCode: row.ProductCode, ProductName: row.ProductName, PriceMinor: row.PriceMinor, Currency: row.Currency})
	}
	return items, total, nil
}

func (reader *ServicePeriodHistoryReader) ListServicePeriodHistoryEntitlements(ctx context.Context, definitionID int64, limit, offset int32) ([]productport.ServicePeriodHistoryEntitlement, int64, error) {
	if definitionID < 1 || !validServicePeriodHistoryPage(limit, offset) {
		return nil, 0, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountServicePeriodHistoryEntitlements(ctx, definitionID)
	if err != nil || total < 0 {
		return nil, 0, productport.ErrServicePeriodHistoryUnavailable
	}
	rows, err := queries.ListServicePeriodHistoryEntitlements(ctx, productdb.ListServicePeriodHistoryEntitlementsParams{DefinitionID: definitionID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, productport.ErrServicePeriodHistoryUnavailable
	}
	items := make([]productport.ServicePeriodHistoryEntitlement, 0, len(rows))
	for _, row := range rows {
		item, err := servicePeriodHistoryEntitlement(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func (reader *ServicePeriodHistoryReader) ListServicePeriodHistoryEvents(ctx context.Context, definitionID int64, limit, offset int32) ([]productport.ServicePeriodHistoryEvent, int64, error) {
	if definitionID < 1 || !validServicePeriodHistoryPage(limit, offset) {
		return nil, 0, productport.ErrServicePeriodHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountServicePeriodHistoryEvents(ctx, definitionID)
	if err != nil || total < 0 {
		return nil, 0, productport.ErrServicePeriodHistoryUnavailable
	}
	rows, err := queries.ListServicePeriodHistoryEvents(ctx, productdb.ListServicePeriodHistoryEventsParams{DefinitionID: definitionID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, productport.ErrServicePeriodHistoryUnavailable
	}
	items := make([]productport.ServicePeriodHistoryEvent, 0, len(rows))
	for _, row := range rows {
		item, err := servicePeriodHistoryEvent(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func (store *ServicePeriodHistoryStore) queries(ctx context.Context) (*productdb.Queries, error) {
	if store == nil || store.tx == nil || ctx == nil || ctx.Err() != nil {
		return nil, productport.ErrServicePeriodHistoryUnavailable
	}
	tx, err := store.tx(ctx)
	if err != nil || tx == nil {
		return nil, productport.ErrServicePeriodHistoryUnavailable
	}
	return productdb.New(tx), nil
}

func (reader *ServicePeriodHistoryReader) queries(ctx context.Context) (*productdb.Queries, error) {
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, productport.ErrServicePeriodHistoryUnavailable
	}
	return productdb.New(reader.db), nil
}

func validServicePeriodHistoryDefinitionInput(value productport.ServicePeriodHistoryDefinition) bool {
	return value.ID == 0 && value.SourceDefinitionID > 0 && value.ProductID > 0 && value.MembershipConfigID != "" && value.MembershipConfigName != "" && validServicePeriodHistoryTimes(value.CreatedAt, value.UpdatedAt)
}

func validServicePeriodHistoryEntitlementInput(value productport.ServicePeriodHistoryEntitlement) bool {
	return value.ID == 0 && value.SourceEntitlementID > 0 && value.DefinitionID > 0 && optionalServicePeriodHistoryID(value.CustomerID) && optionalServicePeriodHistoryID(value.LastOrderID) &&
		value.MembershipConfigID != "" && value.Status != "" && !value.StartAt.IsZero() && !value.EndAt.IsZero() && validServicePeriodHistoryTimes(value.CreatedAt, value.UpdatedAt)
}

func validServicePeriodHistoryEventInput(value productport.ServicePeriodHistoryEvent) bool {
	return value.ID == 0 && value.SourceEventID > 0 && value.DefinitionID > 0 && optionalServicePeriodHistoryID(value.EntitlementID) && optionalServicePeriodHistoryID(value.CustomerID) && optionalServicePeriodHistoryID(value.OrderID) &&
		value.EventID != "" && value.EventType != "" && !value.CreatedAt.IsZero() && optionalServicePeriodHistoryTime(value.BeforeStartAt) && optionalServicePeriodHistoryTime(value.BeforeEndAt) && optionalServicePeriodHistoryTime(value.AfterStartAt) && optionalServicePeriodHistoryTime(value.AfterEndAt)
}

func validServicePeriodHistoryTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created)
}

func optionalServicePeriodHistoryID(value *int64) bool       { return value == nil || *value > 0 }
func optionalServicePeriodHistoryTime(value *time.Time) bool { return value == nil || !value.IsZero() }
func validServicePeriodHistoryPage(limit, offset int32) bool {
	return limit >= 1 && limit <= 100 && offset >= 0
}

func servicePeriodHistoryDefinition(row productdb.ProductServicePeriodHistory) (productport.ServicePeriodHistoryDefinition, error) {
	created, err := servicePeriodHistoryTime(row.CreatedAt)
	if err != nil {
		return productport.ServicePeriodHistoryDefinition{}, err
	}
	updated, err := servicePeriodHistoryTime(row.UpdatedAt)
	if err != nil || row.ID < 1 || row.SourceDefinitionID < 1 || row.ProductID < 1 || row.MembershipConfigID == "" || row.MembershipConfigName == "" || updated.Before(created) {
		return productport.ServicePeriodHistoryDefinition{}, productport.ErrServicePeriodHistoryUnavailable
	}
	return productport.ServicePeriodHistoryDefinition{ID: row.ID, SourceDefinitionID: row.SourceDefinitionID, ProductID: row.ProductID, MembershipConfigID: row.MembershipConfigID, MembershipConfigName: row.MembershipConfigName, DurationDays: row.DurationDays, Deleted: row.Deleted, CreatedAt: created, UpdatedAt: updated}, nil
}

func servicePeriodHistoryEntitlement(row productdb.ProductServicePeriodEntitlementHistory) (productport.ServicePeriodHistoryEntitlement, error) {
	start, err := servicePeriodHistoryTime(row.StartAt)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	end, err := servicePeriodHistoryTime(row.EndAt)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	created, err := servicePeriodHistoryTime(row.CreatedAt)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	updated, err := servicePeriodHistoryTime(row.UpdatedAt)
	if err != nil || row.ID < 1 || row.SourceEntitlementID < 1 || row.DefinitionID < 1 || row.MembershipConfigID == "" || row.Status == "" || updated.Before(created) {
		return productport.ServicePeriodHistoryEntitlement{}, productport.ErrServicePeriodHistoryUnavailable
	}
	customer, err := servicePeriodHistoryOptionalInt64(row.CustomerID)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	order, err := servicePeriodHistoryOptionalInt64(row.LastOrderID)
	if err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	return productport.ServicePeriodHistoryEntitlement{ID: row.ID, SourceEntitlementID: row.SourceEntitlementID, DefinitionID: row.DefinitionID, CustomerID: customer, MembershipConfigID: row.MembershipConfigID, Status: row.Status, StartAt: start, EndAt: end, LastOrderID: order, LastOutTradeNo: row.LastOutTradeNo, RenewalCount: row.RenewalCount, CreatedAt: created, UpdatedAt: updated}, nil
}

func servicePeriodHistoryEvent(row productdb.ProductServicePeriodEventHistory) (productport.ServicePeriodHistoryEvent, error) {
	created, err := servicePeriodHistoryTime(row.CreatedAt)
	if err != nil || row.ID < 1 || row.SourceEventID < 1 || row.DefinitionID < 1 || row.EventID == "" || row.EventType == "" {
		return productport.ServicePeriodHistoryEvent{}, productport.ErrServicePeriodHistoryUnavailable
	}
	entitlement, err := servicePeriodHistoryOptionalInt64(row.EntitlementID)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	customer, err := servicePeriodHistoryOptionalInt64(row.CustomerID)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	order, err := servicePeriodHistoryOptionalInt64(row.OrderID)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	beforeStart, err := servicePeriodHistoryOptionalTime(row.BeforeStartAt)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	beforeEnd, err := servicePeriodHistoryOptionalTime(row.BeforeEndAt)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	afterStart, err := servicePeriodHistoryOptionalTime(row.AfterStartAt)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	afterEnd, err := servicePeriodHistoryOptionalTime(row.AfterEndAt)
	if err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	return productport.ServicePeriodHistoryEvent{ID: row.ID, SourceEventID: row.SourceEventID, DefinitionID: row.DefinitionID, EntitlementID: entitlement, CustomerID: customer, OrderID: order, EventID: row.EventID, EventType: row.EventType, DurationDays: row.DurationDays, OutTradeNo: row.OutTradeNo, BeforeStartAt: beforeStart, BeforeEndAt: beforeEnd, AfterStartAt: afterStart, AfterEndAt: afterEnd, CreatedAt: created}, nil
}

func servicePeriodHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func servicePeriodHistoryNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return servicePeriodHistoryTimestamp(*value)
}

func servicePeriodHistoryNullableInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func servicePeriodHistoryTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite || value.Time.IsZero() {
		return time.Time{}, productport.ErrServicePeriodHistoryUnavailable
	}
	return value.Time.UTC().Truncate(time.Microsecond), nil
}

func servicePeriodHistoryOptionalTime(value pgtype.Timestamptz) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	result, err := servicePeriodHistoryTime(value)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func servicePeriodHistoryOptionalInt64(value pgtype.Int8) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.Int64 < 1 {
		return nil, productport.ErrServicePeriodHistoryUnavailable
	}
	result := value.Int64
	return &result, nil
}

func servicePeriodHistoryWriteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || errors.As(err, &postgresError) && strings.HasPrefix(postgresError.Code, "23") {
		return productport.ErrServicePeriodHistoryConflict
	}
	return productport.ErrServicePeriodHistoryUnavailable
}

func servicePeriodHistoryReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return productport.ErrServicePeriodHistoryConflict
	}
	return productport.ErrServicePeriodHistoryUnavailable
}
