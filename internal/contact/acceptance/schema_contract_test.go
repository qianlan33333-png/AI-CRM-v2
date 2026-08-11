package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

var forbiddenCustomerIdentityColumns = []string{
	"external_userid", "unionid", "openid", "phone", "mobile",
}

func TestContactCoreMigrationFreezesTablesIndexesAndOneID(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "migrations", "00005_contact_core.sql")
	contents, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	lower := strings.ToLower(string(contents))
	for _, table := range []string{"staff", "channels", "customers", "tag_groups", "tags", "customer_tags"} {
		if !strings.Contains(lower, "create table "+table+" (") {
			t.Fatalf("migration missing table %s", table)
		}
	}
	customerDDL := between(t, lower, "create table customers (", ");")
	for _, forbidden := range forbiddenCustomerIdentityColumns {
		if strings.Contains(customerDDL, forbidden) {
			t.Fatalf("customers DDL leaks external identity column %s", forbidden)
		}
	}
	for _, index := range expectedIndexes() {
		if !strings.Contains(lower, "create index "+index) {
			t.Fatalf("migration missing index %s", index)
		}
	}
	if !strings.Contains(lower, "create extension if not exists pg_trgm") {
		t.Fatal("migration must provision pg_trgm for indexed keyword search")
	}
}

func TestInternalCustomerQueryContractStaysChannelNeutralAndKeysetOnly(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(contactapp.CustomerRecord{}),
		reflect.TypeOf(contactapp.CustomerListQuery{}),
	} {
		for index := 0; index < typ.NumField(); index++ {
			name := strings.ToLower(typ.Field(index).Name)
			for _, forbidden := range forbiddenCustomerIdentityColumns {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s leaks external identity field %s", typ.Name(), forbidden)
				}
			}
			if strings.Contains(name, "offset") {
				t.Fatalf("%s reintroduced offset pagination", typ.Name())
			}
		}
	}
	queryType := reflect.TypeOf(contactapp.CustomerListQuery{})
	for _, field := range []string{"Watermark", "AfterUpdatedAt", "AfterID", "Limit", "TagID"} {
		if _, ok := queryType.FieldByName(field); !ok {
			t.Fatalf("CustomerListQuery missing %s", field)
		}
	}
	if contactapp.CustomerListExactTotalCap != 10_000 || contactapp.CustomerListMaximumLimit != 200 {
		t.Fatal("customer list caps drifted")
	}
	if _, ok := reflect.TypeOf(contactapp.CustomerListStoreResult{}).FieldByName("HasMore"); !ok {
		t.Fatal("CustomerListStoreResult must expose keyset continuation without leaking limit+1")
	}
}

func expectedIndexes() []string {
	return []string{
		"idx_customer_tags_tag", "idx_customers_added_keyset", "idx_customers_channel_keyset",
		"idx_customers_deleted_keyset", "idx_customers_interact_keyset", "idx_customers_name_trgm",
		"idx_customers_owner_keyset", "idx_customers_stage_keyset", "idx_tags_catalog",
	}
}

func between(t *testing.T, value, start, end string) string {
	t.Helper()
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		t.Fatalf("missing start marker %q", start)
	}
	value = value[startIndex+len(start):]
	endIndex := strings.Index(value, end)
	if endIndex < 0 {
		t.Fatalf("missing end marker %q", end)
	}
	return value[:endIndex]
}
