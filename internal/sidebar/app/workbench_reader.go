package app

import (
	"context"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	"golang.org/x/sync/errgroup"
)

type WorkbenchCounts struct {
	Questionnaires int
	Orders         int64
	PeriodicOrders int
	Materials      int64
}

type WorkbenchReader interface {
	Read(context.Context, contactport.CustomerID) (WorkbenchCounts, error)
}

type questionnaireCounter interface {
	CountCustomerSurveyAnswers(context.Context, contactport.CustomerID) (int, error)
}

type orderCounter interface {
	CountCustomer(context.Context, int64) (int64, error)
}

type memberCounter interface {
	CountCustomer(context.Context, int64) (int, error)
}

type mediaCounter interface {
	CountEnabledImages(context.Context) (int64, error)
}

type domainWorkbenchReader struct {
	surveys surveyport.CustomerSurveyAnswerReader
	orders  orderport.Query
	members MemberApplication
	media   mediaport.ImageLibraryReader
}

func (reader domainWorkbenchReader) Read(ctx context.Context, customerID contactport.CustomerID) (WorkbenchCounts, error) {
	if ctx == nil || customerID < 1 {
		return WorkbenchCounts{}, ErrInvalidInput
	}
	group, groupContext := errgroup.WithContext(ctx)
	counts := WorkbenchCounts{}
	group.Go(func() error {
		if counter, ok := reader.surveys.(questionnaireCounter); ok {
			count, err := counter.CountCustomerSurveyAnswers(groupContext, customerID)
			counts.Questionnaires = count
			return mapDependencyError(err)
		}
		page, err := reader.surveys.ListCustomerSurveyAnswers(groupContext, customerID, 20)
		counts.Questionnaires = len(page.Items)
		return mapDependencyError(err)
	})
	group.Go(func() error {
		if counter, ok := reader.orders.(orderCounter); ok {
			count, err := counter.CountCustomer(groupContext, int64(customerID))
			counts.Orders = count
			return mapDependencyError(err)
		}
		id := int64(customerID)
		page, err := reader.orders.List(groupContext, orderport.Filter{CustomerID: &id, Limit: 1})
		counts.Orders = page.Total
		return mapDependencyError(err)
	})
	group.Go(func() error {
		if counter, ok := reader.members.(memberCounter); ok {
			count, err := counter.CountCustomer(groupContext, int64(customerID))
			counts.PeriodicOrders = count
			return mapDependencyError(err)
		}
		page, err := reader.members.ListCustomer(groupContext, PeriodicListQuery{CustomerID: int64(customerID), Limit: 100})
		counts.PeriodicOrders = len(page.Items)
		return mapDependencyError(err)
	})
	group.Go(func() error {
		if counter, ok := reader.media.(mediaCounter); ok {
			count, err := counter.CountEnabledImages(groupContext)
			counts.Materials = count
			return mapDependencyError(err)
		}
		page, err := reader.media.ListImages(groupContext, mediaport.ImageListQuery{Limit: 1, EnabledOnly: true})
		counts.Materials = page.Total
		return mapDependencyError(err)
	})
	if err := group.Wait(); err != nil {
		return WorkbenchCounts{}, mapDependencyError(err)
	}
	if counts.Questionnaires < 0 || counts.Orders < 0 || counts.PeriodicOrders < 0 || counts.Materials < 0 {
		return WorkbenchCounts{}, ErrUnavailable
	}
	return counts, nil
}
