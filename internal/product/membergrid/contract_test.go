package membergrid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestDTOJSONFieldWhitelists(t *testing.T) {
	assertJSONFields(t, AccessResponse{}, []string{
		"can_edit", "can_manage_views", "can_query", "can_share", "can_view", "product_id",
	})
	assertJSONFields(t, MemberRow{}, []string{
		"customer_id", "display_name", "expired_at", "expires_at", "member_ref", "removed_at", "service_product_id", "source", "starts_at", "state", "updated_at", "version",
	})
	assertJSONFields(t, QueryResponse{}, []string{
		"has_more", "limit", "next_cursor", "rows",
	})
	assertJSONFields(t, MemberView{}, []string{
		"id", "name", "read_only", "source",
	})
	assertJSONFields(t, ColumnDefinition{}, []string{
		"key", "label", "nullable", "type",
	})
}

func TestSchemaAndViewsContainNoOpaqueOrMutableMetadata(t *testing.T) {
	schema := SchemaResponse{ServiceProductID: 1, Columns: cloneColumns(safeColumns)}
	views := MemberViewsResponse{ProductID: 1, Views: append([]MemberView(nil), builtInViews...)}
	encoded, err := json.Marshal(struct {
		Schema SchemaResponse      `json:"schema"`
		Views  MemberViewsResponse `json:"views"`
	}{Schema: schema, Views: views})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"db_column", "database", "table_name", "opaque", "metadata", "sql", "collaborator",
		"external_share", "public_token", "editable", "unionid", "external_userid", "remark", "alliance", "mobile",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metadata leaked %q: %s", forbidden, body)
		}
	}
	if len(views.Views) != 1 || views.Views[0].Source != "built_in" || !views.Views[0].ReadOnly {
		t.Fatalf("views=%+v", views)
	}
}

func TestRepositorySQLIsReadOnlyExactJoinAndKeyset(t *testing.T) {
	for name, query := range map[string]string{"first": firstPageSQL, "after": afterPageSQL} {
		lower := strings.ToLower(query)
		if !strings.Contains(lower, "join public.customers as c on c.id = m.customer_id") {
			t.Fatalf("%s query lacks exact customer foreign-key join: %s", name, query)
		}
		if !strings.Contains(lower, "where m.service_product_id = $1") ||
			!strings.Contains(lower, "order by m.updated_at desc, m.member_ref desc") {
			t.Fatalf("%s query lacks product predicate or stable order: %s", name, query)
		}
		for _, forbidden := range []string{
			" offset ", "unionid", "external_userid", "order_id", "granted_by", "revoked_by",
			"receipt", "provider", "mobile", "remark", "alliance", "c.extra", "insert ", "update ", "delete ", "merge ", "copy ", "call ",
		} {
			if strings.Contains(" "+lower+" ", forbidden) {
				t.Fatalf("%s query contains forbidden token %q: %s", name, forbidden, query)
			}
		}
	}
	if !strings.Contains(strings.ToLower(afterPageSQL), "(m.updated_at, m.member_ref) < ($4::timestamptz, $5::text)") {
		t.Fatalf("after query does not use the composite server keyset: %s", afterPageSQL)
	}
}

func TestProductionPackageHasNoExternalClientOrForbiddenDomainDependency(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate package")
	}
	directory := filepath.Dir(currentFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		lower := strings.ToLower(string(contents))
		for _, forbidden := range []string{
			"internal/identity/", "internal/wecom/", "internal/outbound/", "internal/order/",
			"http.client{", "http.newrequest", ".do(request", "net.dial", "grpc.dial",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains forbidden dependency/call %q", entry.Name(), forbidden)
			}
		}
	}
}

func assertJSONFields(t *testing.T, value any, expected []string) {
	t.Helper()
	typeOf := reflect.TypeOf(value)
	actual := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		tag := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			t.Fatalf("%s.%s lacks a closed JSON field", typeOf.Name(), typeOf.Field(index).Name)
		}
		actual = append(actual, tag)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("%s fields=%v want=%v", typeOf.Name(), actual, expected)
	}
}
