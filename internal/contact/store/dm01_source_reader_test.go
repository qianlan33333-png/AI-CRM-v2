package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	legacysourcedb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/legacysource/generated"
)

func TestDM01SourceReaderRejectsManifestBeforeDatabaseAccess(t *testing.T) {
	reader := NewDM01SourceReader(nil)
	called := false
	err := reader.WithSnapshot(context.Background(), migration.Manifest{}, func(migration.SourceSnapshot) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("err = %v, called = %v", err, called)
	}
}

func TestDM01SourceSchemaPreflightComparesCanonicalDigest(t *testing.T) {
	rows := []legacysourcedb.ListDM01SourceColumnsRow{{Ordinal: 1, ColumnName: "id", DataType: "bigint", NotNull: true}}
	digest, err := migration.CanonicalSchemaDigest([]migration.SourceColumn{{Ordinal: 1, Name: "id", DataType: "bigint", NotNull: true}})
	if err != nil {
		t.Fatal(err)
	}
	table := migration.Table{Name: "people", SchemaDigest: digest}
	if err := validateDM01SourceSchema(table, rows); err != nil {
		t.Fatal(err)
	}
	table.SchemaDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := validateDM01SourceSchema(table, rows); !errors.Is(err, migration.ErrSourceSchemaDrift) {
		t.Fatalf("schema mismatch error = %v", err)
	}
}

func TestDM01IdentityProjectionRequiresExplicitScope(t *testing.T) {
	if unionID, openIDs := dm01IdentityProjection(migration.Manifest{}); unionID || openIDs {
		t.Fatal("unscoped identity fields were projected")
	}
	if unionID, openIDs := dm01IdentityProjection(migration.Manifest{OpenPlatformAccount: "account"}); !unionID || openIDs {
		t.Fatal("unionid scope did not remain independent")
	}
	if unionID, openIDs := dm01IdentityProjection(migration.Manifest{WeChatAppScopes: map[string]string{"wx1": "account"}}); unionID || !openIDs {
		t.Fatal("openid scope did not remain independent")
	}
}

func TestDM01SnapshotRejectsAlteredBound(t *testing.T) {
	bound := migration.SourceUpperBound{Table: "owner_role_map", Watermark: time.Unix(1, 0).UTC(), SourceKey: "owner"}
	snapshot := &dm01SourceSnapshot{bounds: []migration.SourceUpperBound{bound}, queries: legacysourcedb.New(nil)}
	bound.SourceKey = "other"
	if err := snapshot.validateBound("owner_role_map", bound, true); !errors.Is(err, migration.ErrSourceSchemaDrift) {
		t.Fatalf("altered bound error = %v", err)
	}
}
