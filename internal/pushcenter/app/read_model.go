// Package app implements the read-only Push Center compatibility projection.
// It never accepts work, invokes a provider, or controls a queue.
package app

import (
	"context"
	"errors"
	"reflect"
	"strings"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const CapabilityOwner = "ai_crm_next/platform_foundation/push_center"

var (
	ErrReadModelUnavailable = errors.New("push center read model unavailable")
	ErrInvalidReadModel     = errors.New("invalid push center read model")
)

// Filter is the legacy route's complete fifteen-field textual filter. It does
// not carry an actor or owner scope: both routes require a global admin read.
type Filter struct {
	Section        string
	EffectType     string
	Status         string
	BusinessType   string
	BusinessID     string
	TargetType     string
	TargetID       string
	ExternalUserID string
	OwnerUserID    string
	TraceID        string
	IdempotencyKey string
	SourceModule   string
	SourceRoute    string
	CreatedFrom    string
	CreatedTo      string
}

// Normalized trims every legacy input before it reaches the query boundary.
func (filter Filter) Normalized() Filter {
	filter.Section = strings.TrimSpace(filter.Section)
	filter.EffectType = strings.TrimSpace(filter.EffectType)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.BusinessType = strings.TrimSpace(filter.BusinessType)
	filter.BusinessID = strings.TrimSpace(filter.BusinessID)
	filter.TargetType = strings.TrimSpace(filter.TargetType)
	filter.TargetID = strings.TrimSpace(filter.TargetID)
	filter.ExternalUserID = strings.TrimSpace(filter.ExternalUserID)
	filter.OwnerUserID = strings.TrimSpace(filter.OwnerUserID)
	filter.TraceID = strings.TrimSpace(filter.TraceID)
	filter.IdempotencyKey = strings.TrimSpace(filter.IdempotencyKey)
	filter.SourceModule = strings.TrimSpace(filter.SourceModule)
	filter.SourceRoute = strings.TrimSpace(filter.SourceRoute)
	filter.CreatedFrom = strings.TrimSpace(filter.CreatedFrom)
	filter.CreatedTo = strings.TrimSpace(filter.CreatedTo)
	return filter
}

// ResponseFilters preserves every non-empty legacy filter for an authenticated
// administrator. Some values are internal PII, so this object must never be
// exposed through a public route.
func (filter Filter) ResponseFilters() map[string]string {
	filter = filter.Normalized()
	result := make(map[string]string, 15)
	for _, field := range []struct{ key, value string }{
		{"section", filter.Section}, {"effect_type", filter.EffectType}, {"status", filter.Status},
		{"business_type", filter.BusinessType}, {"business_id", filter.BusinessID}, {"target_type", filter.TargetType},
		{"target_id", filter.TargetID}, {"external_userid", filter.ExternalUserID}, {"owner_userid", filter.OwnerUserID},
		{"trace_id", filter.TraceID}, {"idempotency_key", filter.IdempotencyKey}, {"source_module", filter.SourceModule},
		{"source_route", filter.SourceRoute}, {"created_from", filter.CreatedFrom}, {"created_to", filter.CreatedTo},
	} {
		if field.value != "" {
			result[field.key] = field.value
		}
	}
	return result
}

type SectionDefinition struct {
	Key           string
	Label         string
	EffectTypes   []string
	CapabilityKey string
}

type StatusDefinition struct {
	Key        string
	Label      string
	Definition string
}

var sectionDefinitions = []SectionDefinition{
	{"questionnaire", "问卷外推", []string{"webhook.questionnaire_submission.push"}, "questionnaire_external_push"},
	{"order", "订单外推", []string{"webhook.order_paid.push"}, "order_paid_push"},
	{"ai_assist", "AI 助手", []string{"ai_assist.campaign.message.plan", "ai_assist.campaign.message.loopback", "wecom.message.private.send"}, "ai_assist_push"},
	{"private_broadcast", "私信群发", []string{"wecom.message.private.send"}, "private_broadcast"},
	{"group_ops", "群自动化运营", []string{"group_ops.message.loopback", "group_ops.webhook.action.loopback", "wecom.message.group.send"}, "group_ops_push"},
	{"group_broadcast", "群群发", []string{"wecom.message.broadcast.send", "wecom.message.group.send"}, "group_broadcast"},
	{"customer_webhook", "客户自动化 Webhook", []string{"webhook.customer_automation.retry", "webhook.customer_automation.retry_due"}, "customer_webhook"},
	{"tags", "企微标签", []string{"wecom.contact.tag.mark", "wecom.contact.tag.unmark", "wecom.profile.update"}, "tags"},
	{"welcome", "欢迎语", []string{"wecom.welcome_message.send"}, "welcome_message"},
	{"payment", "支付查询", []string{"payment.wechat.order.query", "payment.wechat.refund.request", "payment.wechat.refund.query", "payment.alipay.order.query", "payment.alipay.refund.query"}, "payment_query"},
	{"integrations", "集成推送", []string{"feishu.webhook.notify", "openclaw.context.push", "media.storage.upload", "wecom.media.upload", "webhook.generic.push"}, "integrations"},
	{"test_receiver", "测试接收端", []string{}, "test_receiver"},
	{"other", "其他", []string{}, ""},
}

var statusDefinitions = []StatusDefinition{
	{"pending", "待执行", "已进入统一推送任务池；有可用产能时立即领取，否则按所属通道正常排队或等待前置条件。"},
	{"running", "执行中", "任务已被执行 worker 领取，正在执行外部动作。"},
	{"succeeded", "执行成功", "外部动作已执行成功，并已有成功 attempt 记录。"},
	{"sent", "已发送", "主发送链路已完成发送。"},
	{"simulated", "模拟执行", "适配器完成模拟执行，没有发生真实外部调用。"},
	{"unknown_after_dispatch", "结果待核对", "外部调用结果不确定，必须先对账且不会自动重试。"},
	{"failed", "发送失败", "主发送链路未成功完成。"},
	{"sent_with_shadow_warning", "已发送 · 影子链路异常", "主发送链路已完成发送，但影子链路或观测链路存在异常。"},
	{"shadow_failed_not_business_failed", "影子链路失败，未发现主发送记录", "仅发现影子链路失败，未发现对应主发送记录；不计入业务发送失败。"},
}

type Summary struct {
	AppliedFilter     Filter
	Total             int64
	ByStatus          map[string]int64
	ByEffectiveStatus map[string]int64
	BySection         map[string]int64
}

// Repository is the only data seam. Its implementation may read only the
// Push Center projection created by this slice, never another domain's tables.
type Repository interface {
	ReadSummary(context.Context, Filter) (Summary, error)
}

type Service struct {
	uow        platformport.UnitOfWork
	repository Repository
}

func NewService(uow platformport.UnitOfWork, repository Repository) *Service {
	return &Service{uow: uow, repository: repository}
}

// Read returns a verified aggregation. A missing, incomplete, or malformed
// projection never becomes a successful zero result.
func (service *Service) Read(ctx context.Context, filter Filter) (Summary, error) {
	if ctx == nil || ctx.Err() != nil || service == nil || service.uow == nil || service.repository == nil {
		return Summary{}, ErrReadModelUnavailable
	}
	filter = filter.Normalized()
	var result Summary
	if err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var readErr error
		result, readErr = service.repository.ReadSummary(txCtx, filter)
		return readErr
	}); err != nil {
		return Summary{}, errors.Join(ErrReadModelUnavailable, err)
	}
	if err := validateSummary(result, filter); err != nil {
		return Summary{}, errors.Join(ErrReadModelUnavailable, err)
	}
	return cloneSummary(result), nil
}

func validateSummary(summary Summary, filter Filter) error {
	if !reflect.DeepEqual(summary.AppliedFilter.Normalized(), filter) || summary.Total < 0 ||
		summary.ByStatus == nil || summary.ByEffectiveStatus == nil || summary.BySection == nil {
		return ErrInvalidReadModel
	}
	if sum(summary.ByStatus) != summary.Total || sum(summary.ByEffectiveStatus) != summary.Total || sum(summary.BySection) != summary.Total {
		return ErrInvalidReadModel
	}
	for key, count := range summary.ByStatus {
		if !validStatus(key) || count < 0 {
			return ErrInvalidReadModel
		}
	}
	for key, count := range summary.ByEffectiveStatus {
		if !validEffectiveStatus(key) || count < 0 {
			return ErrInvalidReadModel
		}
	}
	for key, count := range summary.BySection {
		if !validSection(key) || count < 0 {
			return ErrInvalidReadModel
		}
	}
	return nil
}

func sum(counts map[string]int64) int64 {
	var total int64
	for _, count := range counts {
		total += count
	}
	return total
}

func validSection(value string) bool {
	for _, definition := range sectionDefinitions {
		if definition.Key == value {
			return true
		}
	}
	return false
}

func validStatus(value string) bool {
	for _, definition := range statusDefinitions {
		if definition.Key == value {
			return true
		}
	}
	return false
}

func validEffectiveStatus(value string) bool { return value == "reconciled" || validStatus(value) }

func SectionDefinitions() []SectionDefinition {
	result := make([]SectionDefinition, len(sectionDefinitions))
	for index, definition := range sectionDefinitions {
		result[index] = definition
		result[index].EffectTypes = append([]string(nil), definition.EffectTypes...)
	}
	return result
}

func StatusDefinitions() []StatusDefinition {
	return append([]StatusDefinition(nil), statusDefinitions...)
}

func cloneSummary(summary Summary) Summary {
	summary.AppliedFilter = summary.AppliedFilter.Normalized()
	summary.ByStatus = cloneCounts(summary.ByStatus)
	summary.ByEffectiveStatus = cloneCounts(summary.ByEffectiveStatus)
	summary.BySection = cloneCounts(summary.BySection)
	return summary
}

func cloneCounts(value map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(value))
	for key, count := range value {
		result[key] = count
	}
	return result
}

// ReadUnavailableErrorClass intentionally identifies the compatibility error
// category without reflecting raw database errors into an administrator page.
func ReadUnavailableErrorClass(error) string { return "ReadModelUnavailableError" }
