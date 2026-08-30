package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func CreateCustomer(ctx context.Context, db queryRower, name string) (int64, error) {
	if db == nil || name == "" {
		return 0, fmt.Errorf("valid Contact customer fixture required")
	}
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO customers(name) VALUES($1) RETURNING id`, name).Scan(&id)
	return id, err
}
