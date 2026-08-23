package legacyaudiencemembers

import (
	"context"
	"errors"

	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

type audienceMemberQueries interface {
	LegacyAudiencePackageExists(context.Context, int64) (bool, error)
	ListLegacyAudienceMembers(context.Context, segmentdb.ListLegacyAudienceMembersParams) ([]segmentdb.ListLegacyAudienceMembersRow, error)
}

var (
	_ PackageExistenceReader = (*SQLRepository)(nil)
	_ MemberRepository       = (*SQLRepository)(nil)
)

type SQLRepository struct {
	queries audienceMemberQueries
}

func NewSQLRepository() *SQLRepository { return &SQLRepository{} }

func newSQLRepository(queries audienceMemberQueries) (*SQLRepository, error) {
	if nilInterface(queries) {
		return nil, ErrUnavailable
	}
	return &SQLRepository{queries: queries}, nil
}

func (repository *SQLRepository) PackageExists(ctx context.Context, packageID int64) (bool, error) {
	if repository == nil || ctx == nil || packageID <= 0 {
		return false, ErrUnavailable
	}
	queries, err := repository.transactionQueries(ctx)
	if err != nil {
		return false, errors.Join(ErrUnavailable, err)
	}
	exists, err := queries.LegacyAudiencePackageExists(ctx, packageID)
	if err != nil {
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
	if repository == nil || ctx == nil || packageID <= 0 ||
		limit < 1 || limit > MaximumLimit || offset < 0 {
		return MemberPage{}, ErrUnavailable
	}
	queries, err := repository.transactionQueries(ctx)
	if err != nil {
		return MemberPage{}, errors.Join(ErrUnavailable, err)
	}
	rows, err := queries.ListLegacyAudienceMembers(ctx, segmentdb.ListLegacyAudienceMembersParams{
		PackageID: packageID,
		RowLimit:  int32(limit),
		RowOffset: offset,
	})
	if err != nil {
		return MemberPage{}, errors.Join(ErrUnavailable, err)
	}
	if len(rows) == 0 {
		return MemberPage{}, ErrUnavailable
	}

	page := MemberPage{Items: make([]MemberRecord, 0, limit)}
	for index, row := range rows {
		if row.Total < 0 || (index > 0 && row.Total != page.Total) {
			return MemberPage{}, ErrUnavailable
		}
		page.Total = row.Total
		if !row.CustomerID.Valid {
			if row.Name.Valid || row.ComputedAt.Valid {
				return MemberPage{}, ErrUnavailable
			}
			continue
		}
		if row.CustomerID.Int64 <= 0 || !row.Name.Valid || !row.ComputedAt.Valid || row.ComputedAt.Time.IsZero() {
			return MemberPage{}, ErrUnavailable
		}
		page.Items = append(page.Items, MemberRecord{
			CustomerID: row.CustomerID.Int64,
			Nickname:   row.Name.String,
			EnteredAt:  row.ComputedAt.Time.UTC(),
		})
	}
	return page, nil
}

func (repository *SQLRepository) transactionQueries(ctx context.Context) (audienceMemberQueries, error) {
	if repository == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	if !nilInterface(repository.queries) {
		return repository.queries, nil
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return segmentdb.New(tx), nil
}
