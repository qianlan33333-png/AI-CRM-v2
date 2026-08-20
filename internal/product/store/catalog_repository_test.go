package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
)

func TestUnavailableClassifiesNotFoundAndUniqueOrderConflict(t *testing.T) {
	if got := unavailable(pgx.ErrNoRows); !errors.Is(got, productapp.ErrNotFound) {
		t.Fatalf("pgx no rows=%v, want product not found sentinel", got)
	}
	if got := unavailable(&pgconn.PgError{Code: "23505", ConstraintName: "product_local_entitlements_order"}); !errors.Is(got, productapp.ErrConflict) {
		t.Fatalf("duplicate local entitlement order=%v, want conflict sentinel", got)
	}
}
