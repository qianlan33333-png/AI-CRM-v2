package legacyaudiencemembers

import (
	"context"
	"database/sql"
	"errors"
)

const packageExistsSQL = `SELECT EXISTS (
  SELECT 1
  FROM public.segments AS segment
  JOIN public.ai_audience_package_metadata AS metadata
    ON metadata.segment_id = segment.id
  WHERE segment.id = $1
)`

// memberPageSQL returns the total and requested page from one PostgreSQL
// statement so a concurrent atomic Segment refresh cannot split the count and
// rows across different committed snapshots. The LEFT JOIN emits one total-only
// row for an empty or past-the-end page.
const memberPageSQL = `WITH requested_page AS (
  SELECT
    member.customer_id,
    customer.name,
    member.computed_at
  FROM public.segment_members AS member
  JOIN public.customers AS customer
    ON customer.id = member.customer_id
  WHERE member.segment_id = $1
  ORDER BY member.computed_at DESC, member.customer_id DESC
  LIMIT $2 OFFSET $3
), snapshot_total AS (
  SELECT count(*)::bigint AS total
  FROM public.segment_members
  WHERE segment_id = $1
)
SELECT
  snapshot_total.total,
  requested_page.customer_id,
  requested_page.name,
  requested_page.computed_at
FROM snapshot_total
LEFT JOIN requested_page ON TRUE
ORDER BY requested_page.computed_at DESC NULLS LAST,
         requested_page.customer_id DESC NULLS LAST`

type SQLRow interface {
	Scan(...any) error
}

type SQLRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

type SQLReader interface {
	QueryRow(context.Context, string, ...any) SQLRow
	Query(context.Context, string, ...any) (SQLRows, error)
}

// SQLProvider supplies a local read executor. It must not open a write
// transaction or make an external call.
type SQLProvider interface {
	Reader(context.Context) (SQLReader, error)
}

var (
	_ PackageExistenceReader = (*SQLRepository)(nil)
	_ MemberRepository       = (*SQLRepository)(nil)
)

type SQLRepository struct {
	provider SQLProvider
}

func NewSQLRepository(provider SQLProvider) (*SQLRepository, error) {
	if nilInterface(provider) {
		return nil, ErrUnavailable
	}
	return &SQLRepository{provider: provider}, nil
}

func (repository *SQLRepository) PackageExists(ctx context.Context, packageID int64) (bool, error) {
	if repository == nil || nilInterface(repository.provider) || ctx == nil || packageID <= 0 {
		return false, ErrUnavailable
	}
	reader, err := repository.provider.Reader(ctx)
	if err != nil || nilInterface(reader) {
		return false, errors.Join(ErrUnavailable, err)
	}
	var exists bool
	if err = reader.QueryRow(ctx, packageExistsSQL, packageID).Scan(&exists); err != nil {
		return false, errors.Join(ErrUnavailable, err)
	}
	return exists, nil
}

func (repository *SQLRepository) ListMembers(
	ctx context.Context,
	packageID int64,
	limit int,
	offset int64,
) (MemberPage, error) {
	if repository == nil || nilInterface(repository.provider) || ctx == nil || packageID <= 0 ||
		limit < 1 || limit > MaximumLimit || offset < 0 {
		return MemberPage{}, ErrUnavailable
	}
	reader, err := repository.provider.Reader(ctx)
	if err != nil || nilInterface(reader) {
		return MemberPage{}, errors.Join(ErrUnavailable, err)
	}

	rows, err := reader.Query(ctx, memberPageSQL, packageID, limit, offset)
	if err != nil {
		return MemberPage{}, errors.Join(ErrUnavailable, err)
	}
	if nilInterface(rows) {
		return MemberPage{}, ErrUnavailable
	}
	defer rows.Close()

	page := MemberPage{Items: make([]MemberRecord, 0, limit)}
	totalSeen := false
	for rows.Next() {
		var (
			total      int64
			customerID sql.NullInt64
			nickname   sql.NullString
			enteredAt  sql.NullTime
		)
		if err = rows.Scan(&total, &customerID, &nickname, &enteredAt); err != nil {
			return MemberPage{}, errors.Join(ErrUnavailable, err)
		}
		if total < 0 || (totalSeen && total != page.Total) {
			return MemberPage{}, ErrUnavailable
		}
		page.Total, totalSeen = total, true
		if !customerID.Valid {
			if nickname.Valid || enteredAt.Valid {
				return MemberPage{}, ErrUnavailable
			}
			continue
		}
		if !nickname.Valid || !enteredAt.Valid {
			return MemberPage{}, ErrUnavailable
		}
		page.Items = append(page.Items, MemberRecord{
			CustomerID: customerID.Int64,
			Nickname:   nickname.String,
			EnteredAt:  enteredAt.Time.UTC(),
		})
	}
	if err = rows.Err(); err != nil {
		return MemberPage{}, errors.Join(ErrUnavailable, err)
	}
	if !totalSeen {
		return MemberPage{}, ErrUnavailable
	}
	return page, nil
}
