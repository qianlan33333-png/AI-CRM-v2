package store

import (
	"context"
	"errors"
	"reflect"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errInvalidTagCatalogRow = errors.New("tag catalog query returned an invalid row")

// TagCatalogRepository serves contact-owned tag catalog reads through the
// transaction-bound unit-of-work context.
type TagCatalogRepository struct{}

var _ contactapp.TagCatalogStore = (*TagCatalogRepository)(nil)

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
