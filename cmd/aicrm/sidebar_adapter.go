package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

type sidebarCorpReader struct {
	settings              configport.Service
	fallback              string
	fallbackAuthoritative bool
}

func (reader sidebarCorpReader) CorpID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", configport.ErrInvalidSetting
	}
	if reader.fallbackAuthoritative {
		value := strings.TrimSpace(reader.fallback)
		if value == "" || value != reader.fallback {
			return "", configport.ErrInvalidSetting
		}
		return value, nil
	}
	if reader.settings == nil {
		return "", configport.ErrInvalidSetting
	}
	setting, err := reader.settings.Get(ctx, configport.WeComCorpID)
	if errors.Is(err, configport.ErrSettingNotFound) {
		value := strings.TrimSpace(reader.fallback)
		if value == "" {
			return "", configport.ErrSettingNotFound
		}
		return value, nil
	}
	if err != nil {
		return "", err
	}
	var value string
	if json.Unmarshal(setting.Value, &value) != nil || strings.TrimSpace(value) != value || value == "" {
		return "", configport.ErrInvalidSetting
	}
	return value, nil
}

type sidebarPhoneSource interface {
	Bind(context.Context, identityport.BindCommand) (identityport.BindResult, error)
}

type sidebarPhoneAdapter struct{ source sidebarPhoneSource }

func (adapter sidebarPhoneAdapter) BindPhone(ctx context.Context, command sidebarapp.PhoneBindingCommand) (string, error) {
	if adapter.source == nil {
		return "", sidebarapp.ErrUnavailable
	}
	result, err := adapter.source.Bind(ctx, identityport.BindCommand{
		CustomerID: contactport.CustomerID(command.CustomerID),
		Ref:        identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: command.Mobile, Assurance: identityport.AssuranceDeclared, Source: "sidebar.phone_binding"},
		Actor:      contactport.Actor("admin:" + sidebarInt64String(command.ActorID)), IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		switch {
		case errors.Is(err, identityapp.ErrInvalidIdentity):
			return "", sidebarapp.ErrInvalidInput
		case errors.Is(err, identityapp.ErrIdentityBindIdempotencyConflict):
			return "", sidebarapp.ErrConflict
		default:
			return "", sidebarapp.ErrUnavailable
		}
	}
	return string(result.Status), nil
}

func sidebarInt64String(value int64) string {
	return strconv.FormatInt(value, 10)
}

type sidebarMemberSource interface {
	Get(context.Context, int64, string) (memberdomain.Member, error)
	UpdateFields(context.Context, memberport.UpdateFieldsCommand) (memberdomain.Member, error)
	ListCustomer(context.Context, memberport.CustomerListQuery) (memberport.CustomerListResult, error)
}

type sidebarMemberAdapter struct{ source sidebarMemberSource }

func (adapter sidebarMemberAdapter) Get(ctx context.Context, serviceProductID int64, memberRef string) (sidebarapp.PeriodicMember, error) {
	if adapter.source == nil {
		return sidebarapp.PeriodicMember{}, sidebarapp.ErrUnavailable
	}
	member, err := adapter.source.Get(ctx, serviceProductID, memberRef)
	return sidebarPeriodicMember(member), sidebarMemberError(err)
}

func (adapter sidebarMemberAdapter) UpdateRemark(ctx context.Context, command sidebarapp.PeriodicRemarkCommand) (sidebarapp.PeriodicMember, error) {
	if adapter.source == nil {
		return sidebarapp.PeriodicMember{}, sidebarapp.ErrUnavailable
	}
	member, err := adapter.source.UpdateFields(ctx, memberport.UpdateFieldsCommand{
		ServiceProductID: command.ServiceProductID, MemberRef: command.MemberRef, ExpectedVersion: command.ExpectedVersion,
		Remark: command.Remark, Alliance: command.Alliance, ActorID: command.ActorID, IdempotencyKey: command.IdempotencyKey,
	})
	return sidebarPeriodicMember(member), sidebarMemberError(err)
}

func (adapter sidebarMemberAdapter) ListCustomer(ctx context.Context, query sidebarapp.PeriodicListQuery) (sidebarapp.PeriodicListResult, error) {
	if adapter.source == nil {
		return sidebarapp.PeriodicListResult{}, sidebarapp.ErrUnavailable
	}
	page, err := adapter.source.ListCustomer(ctx, memberport.CustomerListQuery{CustomerID: query.CustomerID, Limit: query.Limit, Offset: query.Offset})
	if err != nil {
		return sidebarapp.PeriodicListResult{}, sidebarMemberError(err)
	}
	items := make([]sidebarapp.PeriodicMember, len(page.Items))
	for index, member := range page.Items {
		items[index] = sidebarPeriodicMember(member)
	}
	return sidebarapp.PeriodicListResult{Items: items, Limit: page.Limit, Offset: page.Offset, HasMore: page.HasMore}, nil
}

func sidebarPeriodicMember(member memberdomain.Member) sidebarapp.PeriodicMember {
	return sidebarapp.PeriodicMember{
		MemberRef: member.MemberRef, ServiceProductID: member.ServiceProductID, CustomerID: member.CustomerID,
		State: string(member.State), Source: string(member.Source), StartsAt: member.StartsAt, ExpiresAt: member.ExpiresAt,
		ExpiredAt: member.ExpiredAt, RemovedAt: member.RemovedAt, Remark: member.Remark, Alliance: member.Alliance,
		Version: member.Version, CreatedAt: member.CreatedAt, UpdatedAt: member.UpdatedAt,
	}
}

func sidebarMemberError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, memberport.ErrInvalidInput):
		return sidebarapp.ErrInvalidInput
	case errors.Is(err, memberport.ErrNotFound):
		return sidebarapp.ErrNotFound
	case errors.Is(err, memberport.ErrConflict):
		return sidebarapp.ErrConflict
	default:
		return sidebarapp.ErrUnavailable
	}
}
