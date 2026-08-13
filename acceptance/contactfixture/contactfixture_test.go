package contactfixture

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCreateCustomerReturnsOneIDFromTransaction(t *testing.T) {
	t.Parallel()
	const want = int64(42)
	id, err := CreateCustomer(context.Background(), fixtureTx{row: fixtureRow{id: want}})
	if err != nil {
		t.Fatal(err)
	}
	if id != want {
		t.Fatalf("OneID=%d, want %d", id, want)
	}
}

func TestCreateCustomerRequiresTransaction(t *testing.T) {
	t.Parallel()
	if _, err := CreateCustomer(context.Background(), nil); !errors.Is(err, ErrNilTransaction) {
		t.Fatalf("CreateCustomer nil transaction error=%v, want ErrNilTransaction", err)
	}
}

func TestSoftDeleteCustomerRequiresValidTransactionAndCustomer(t *testing.T) {
	if err := SoftDeleteCustomer(context.Background(), nil, 1); !errors.Is(err, ErrNilTransaction) {
		t.Fatalf("SoftDeleteCustomer nil transaction error=%v, want ErrNilTransaction", err)
	}
	if err := SoftDeleteCustomer(context.Background(), fixtureTx{}, 0); !errors.Is(err, ErrNilTransaction) {
		t.Fatalf("SoftDeleteCustomer zero customer error=%v, want ErrNilTransaction", err)
	}
}

type fixtureTx struct {
	pgx.Tx
	row pgx.Row
}

func (tx fixtureTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }

type fixtureRow struct{ id int64 }

func (row fixtureRow) Scan(destinations ...any) error {
	*destinations[0].(*int64) = row.id
	return nil
}
