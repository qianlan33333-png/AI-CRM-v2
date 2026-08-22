package store

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errInvalidTagCatalogRow = errors.New("tag catalog query returned an invalid row")

// TagCatalogRepository serves contact-owned tag catalog reads through the
// transaction-bound unit-of-work context.
type TagCatalogRepository struct{}

var _ contactapp.TagCatalogStore = (*TagCatalogRepository)(nil)
var _ contactport.TagReferenceReader = (*TagCatalogRepository)(nil)

func NewTagCatalogRepository() *TagCatalogRepository {
	return &TagCatalogRepository{}
}

func (*TagCatalogRepository) ListTags(ctx context.Context) ([]contactapp.TagCatalogRecord, error) {
	if isNilTagCatalogStoreValue(ctx) {
		return nil, platformport.ErrTransactionRequired
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if isNilTagCatalogStoreValue(tx) {
		return nil, platformport.ErrTransactionRequired
	}

	rows, err := contactdb.New(tx).ListTags(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]contactapp.TagCatalogRecord, 0, len(rows))
	for _, row := range rows {
		grouped := row.GroupID.Valid
		if grouped != row.GroupName.Valid || grouped != row.GroupSortOrder.Valid {
			return nil, errInvalidTagCatalogRow
		}

		record := contactapp.TagCatalogRecord{
			ID:        row.ID,
			Name:      row.Name,
			SortOrder: row.SortOrder,
		}
		if grouped {
			record.GroupID = tagCatalogInt64Pointer(row.GroupID.Int64)
			record.GroupName = tagCatalogStringPointer(row.GroupName.String)
			record.GroupSortOrder = tagCatalogInt32Pointer(row.GroupSortOrder.Int32)
		}
		items = append(items, record)
	}
	return items, nil
}

func (*TagCatalogRepository) LockActiveTag(ctx context.Context, id int64) (contactport.TagReference, error) {
	if id < 1 || isNilTagCatalogStoreValue(ctx) {
		return contactport.TagReference{}, contactport.ErrTagReferenceNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil || isNilTagCatalogStoreValue(tx) {
		return contactport.TagReference{}, contactport.ErrTagReferenceUnavailable
	}
	row, err := contactdb.New(tx).LockActiveTagReference(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.TagReference{}, contactport.ErrTagReferenceNotFound
	}
	if err != nil || row.ID != id || row.Name == "" {
		return contactport.TagReference{}, contactport.ErrTagReferenceUnavailable
	}
	result := contactport.TagReference{ID: row.ID, Name: row.Name}
	if row.GroupName.Valid {
		name := row.GroupName.String
		if name == "" {
			return contactport.TagReference{}, contactport.ErrTagReferenceUnavailable
		}
		result.GroupName = &name
	}
	return result, nil
}

func isNilTagCatalogStoreValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func tagCatalogInt64Pointer(value int64) *int64    { return &value }
func tagCatalogInt32Pointer(value int32) *int32    { return &value }
func tagCatalogStringPointer(value string) *string { return &value }
