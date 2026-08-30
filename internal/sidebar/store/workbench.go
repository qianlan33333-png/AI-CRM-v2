package store

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	sidebardb "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/store/generated"
)

type WorkbenchRepository struct {
	pool       *pgxpool.Pool
	wecomScope string
}

func NewWorkbenchRepository(pool *pgxpool.Pool, corpID string) (*WorkbenchRepository, error) {
	corpID = strings.TrimSpace(corpID)
	if pool == nil || corpID == "" {
		return nil, sidebarapp.ErrUnavailable
	}
	return &WorkbenchRepository{pool: pool, wecomScope: "wecom-corp:" + corpID}, nil
}

func (repository *WorkbenchRepository) Read(ctx context.Context, customerID contactport.CustomerID) (sidebarapp.WorkbenchCounts, error) {
	if repository == nil || repository.pool == nil || ctx == nil || customerID < 1 {
		return sidebarapp.WorkbenchCounts{}, sidebarapp.ErrInvalidInput
	}
	row, err := sidebardb.New(repository.pool).ReadSidebarWorkbenchCounts(ctx, sidebardb.ReadSidebarWorkbenchCountsParams{
		WecomScope: repository.wecomScope,
		CustomerID: int64(customerID),
	})
	if err != nil || row.QuestionnaireCount < 0 || row.OrderCount < 0 || row.PeriodicOrderCount < 0 || row.MaterialCount < 0 {
		return sidebarapp.WorkbenchCounts{}, sidebarapp.ErrUnavailable
	}
	maxInt := int64(^uint(0) >> 1)
	if row.QuestionnaireCount > maxInt || row.PeriodicOrderCount > maxInt {
		return sidebarapp.WorkbenchCounts{}, sidebarapp.ErrUnavailable
	}
	return sidebarapp.WorkbenchCounts{
		Questionnaires: int(row.QuestionnaireCount),
		Orders:         row.OrderCount,
		PeriodicOrders: int(row.PeriodicOrderCount),
		Materials:      row.MaterialCount,
	}, nil
}

var _ sidebarapp.WorkbenchReader = (*WorkbenchRepository)(nil)
