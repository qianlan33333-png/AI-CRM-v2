package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const customerContextChatLimit int32 = 20

type customer360Reader interface {
	ReadCustomer360(context.Context, contactport.Customer360ReadInput) (contactport.Customer360Read, error)
}

type customerChatSummaryReader interface {
	ListCustomerChatSummaries(context.Context, wecomport.CustomerChatSummaryQuery) (wecomport.CustomerChatSummaryPage, error)
}

type hxcCurrentReader interface {
	ReadCustomerCurrent(context.Context, contactport.CustomerID) (hxcport.CurrentSnapshot, error)
}

// CustomerContextService composes only local, already-authorized safe read
// ports. It does not generate AI output, invoke a provider, or infer any
// identity linkage.
type CustomerContextService struct {
	customers customer360Reader
	chats     customerChatSummaryReader
	hxc       hxcCurrentReader
}

func NewCustomerContextService(customers customer360Reader, chats customerChatSummaryReader, hxc ...hxcCurrentReader) *CustomerContextService {
	service := &CustomerContextService{customers: customers, chats: chats}
	if len(hxc) == 1 {
		service.hxc = hxc[0]
	}
	return service
}

func (service *CustomerContextService) ReadCustomerContext(
	ctx context.Context,
	query customer360port.CustomerContextQuery,
) (customer360port.CustomerContext, error) {
	if ctx == nil || query.CustomerID <= 0 || (query.OwnerStaffID != nil && *query.OwnerStaffID <= 0) ||
		query.TimelineLimit < 0 || query.TimelineLimit > 200 || len(query.TimelineCursor) > 512 {
		return customer360port.CustomerContext{}, customer360port.ErrInvalidCustomerContext
	}
	if service == nil || nilCustomerContextDependency(service.customers) || nilCustomerContextDependency(service.chats) {
		return customer360port.CustomerContext{}, customer360port.ErrCustomerContextUnavailable
	}
	if err := ctx.Err(); err != nil {
		return customer360port.CustomerContext{}, errors.Join(customer360port.ErrCustomerContextUnavailable, err)
	}

	local, err := service.customers.ReadCustomer360(ctx, contactport.Customer360ReadInput{
		CustomerID: query.CustomerID, OwnerStaffID: cloneCustomerContextInt64(query.OwnerStaffID),
		TimelineCursor: query.TimelineCursor, TimelineLimit: query.TimelineLimit,
	})
	if err != nil {
		if errors.Is(err, contactport.ErrCustomerReadNotFound) {
			return customer360port.CustomerContext{}, contactport.ErrCustomerReadNotFound
		}
		return customer360port.CustomerContext{}, errors.Join(customer360port.ErrCustomerContextUnavailable, err)
	}
	if local.Customer.ID != query.CustomerID {
		return customer360port.CustomerContext{}, customer360port.ErrCustomerContextUnavailable
	}

	result := customerContextFromContact(local)
	service.readHXC(ctx, query.CustomerID, &result)
	page, chatErr := service.chats.ListCustomerChatSummaries(ctx, wecomport.CustomerChatSummaryQuery{
		CustomerID: query.CustomerID, Limit: customerContextChatLimit, Offset: 0,
	})
	if chatErr != nil {
		if errors.Is(chatErr, wecomport.ErrCustomerChatSummaryUnavailable) {
			return result, nil
		}
		return customer360port.CustomerContext{}, errors.Join(customer360port.ErrCustomerContextUnavailable, chatErr)
	}
	if page.Limit != customerContextChatLimit || page.Offset != 0 || page.Total < int64(len(page.Items)) {
		return customer360port.CustomerContext{}, customer360port.ErrCustomerContextUnavailable
	}
	result.Chat = customer360port.ChatSummary{LocalArchiveAvailable: true, Items: make([]customer360port.ChatEntry, len(page.Items)), Total: page.Total}
	for index, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || item.MessageType == "" || item.SentAt.IsZero() {
			return customer360port.CustomerContext{}, customer360port.ErrCustomerContextUnavailable
		}
		result.Chat.Items[index] = customer360port.ChatEntry{ChatType: item.ChatType, MessageType: item.MessageType, SentAt: item.SentAt.UTC()}
	}
	return result, nil
}

func (service *CustomerContextService) readHXC(ctx context.Context, customerID contactport.CustomerID, result *customer360port.CustomerContext) {
	if nilCustomerContextDependency(service.hxc) {
		return
	}
	snapshot, err := service.hxc.ReadCustomerCurrent(ctx, customerID)
	result.HXC.LastSyncedAt = cloneCustomerContextTime(snapshot.LastSyncedAt)
	if err != nil {
		return
	}
	result.HXC.Available = true
	if !snapshot.Found {
		return
	}
	current := snapshot.Current
	consultationRemaining := current.ConsultationLimit - current.ConsultationUsed
	if consultationRemaining < 0 {
		consultationRemaining = 0
	}
	var daysRemaining int32
	if current.SubscriptionExpiresAt != nil {
		days := int32(time.Until(current.SubscriptionExpiresAt.UTC()).Hours() / 24)
		if days > 0 {
			daysRemaining = days
		}
	}
	result.HXC.Status = &customer360port.HXCCurrentStatus{
		SubscriptionTier: current.SubscriptionTier, SubscriptionExpiresAt: cloneCustomerContextTime(current.SubscriptionExpiresAt), DaysRemaining: daysRemaining,
		MonthlyChatQuota: current.MonthlyChatQuota, CurrentPeriodUsed: current.CurrentPeriodUsed,
		ConsultationLimit: current.ConsultationLimit, ConsultationUsed: current.ConsultationUsed, ConsultationRemaining: consultationRemaining,
		Sessions7D: current.Sessions7D, Sessions30D: current.Sessions30D, SessionsTotal: current.SessionsTotal,
		UserMessages7D: current.UserMessages7D, UserMessages30D: current.UserMessages30D, UserMessagesTotal: current.UserMessagesTotal,
		LastUsedAt: cloneCustomerContextTime(current.LastUsedAt), LastCapability: cloneCustomerContextString(current.LastCapability),
		BusinessStage: cloneCustomerContextString(current.BusinessStage), MainLineType: cloneCustomerContextString(current.MainLineType),
		UserSegment: cloneCustomerContextString(current.UserSegment), FocusTopics: append([]string{}, current.FocusTopics...),
		PainTag: cloneCustomerContextString(current.PainTag), SourceUpdatedAt: current.SourceUpdatedAt.UTC(),
	}
}

func customerContextFromContact(local contactport.Customer360Read) customer360port.CustomerContext {
	result := customer360port.CustomerContext{
		Customer: customer360port.Customer{
			ID: local.Customer.ID, Name: local.Customer.Name, StageID: cloneCustomerContextInt64(local.Customer.StageID),
			OwnerStaffID: cloneCustomerContextInt64(local.Customer.OwnerStaffID), ChannelID: cloneCustomerContextInt64(local.Customer.ChannelID),
			AddedAt: cloneCustomerContextTime(local.Customer.AddedAt), LastInteractAt: cloneCustomerContextTime(local.Customer.LastInteractAt),
		},
		Tags:               make([]customer360port.Tag, len(local.Tags)),
		Timeline:           make([]customer360port.TimelineEntry, len(local.Timeline)),
		TimelineNextCursor: cloneCustomerContextString(local.TimelineNextCursor),
		Chat:               customer360port.ChatSummary{LocalArchiveAvailable: false},
	}
	for index, tag := range local.Tags {
		result.Tags[index] = customer360port.Tag{
			ID: tag.ID, GroupID: cloneCustomerContextInt64(tag.GroupID), GroupName: cloneCustomerContextString(tag.GroupName),
			GroupSortOrder: tag.GroupSortOrder, Name: tag.Name, SortOrder: tag.SortOrder,
		}
	}
	for index, event := range local.Timeline {
		result.Timeline[index] = customer360port.TimelineEntry{ID: event.ID, EventType: event.EventType, OccurredAt: event.OccurredAt.UTC()}
	}
	return result
}

func cloneCustomerContextInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCustomerContextString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCustomerContextTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func nilCustomerContextDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}

var _ customer360port.Reader = (*CustomerContextService)(nil)
