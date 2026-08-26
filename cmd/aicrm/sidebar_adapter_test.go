package main

import (
	"context"
	"encoding/json"
	"testing"

	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

type sidebarCorpSettings struct {
	value json.RawMessage
	reads int
}

type sidebarPhoneSourceFake struct {
	command identityport.BindCommand
	result  identityport.BindResult
}

func (source *sidebarPhoneSourceFake) Bind(_ context.Context, command identityport.BindCommand) (identityport.BindResult, error) {
	source.command = command
	return source.result, nil
}

func TestSidebarPhoneAdapterBindsDeclaredPhoneToScopedCustomer(t *testing.T) {
	source := &sidebarPhoneSourceFake{result: identityport.BindResult{Status: identityport.BindBound, CustomerID: 41}}
	status, err := (sidebarPhoneAdapter{source: source}).BindPhone(context.Background(), sidebarapp.PhoneBindingCommand{
		CustomerID: 41, Mobile: "+8613800138000", ActorID: 9, IdempotencyKey: "sidebar-phone-0001",
	})
	if err != nil || status != "bound" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if source.command.CustomerID != 41 || source.command.Ref.Kind != identityport.KindPhone || source.command.Ref.Scope != "phone:e164" ||
		source.command.Ref.Value != "+8613800138000" || source.command.Ref.Assurance != identityport.AssuranceDeclared ||
		source.command.Ref.Source != "sidebar.phone_binding" || source.command.Actor != "admin:9" || source.command.IdempotencyKey != "sidebar-phone-0001" {
		t.Fatalf("unexpected bind command %#v", source.command)
	}
}

func (settings *sidebarCorpSettings) Get(context.Context, configport.Key) (configport.Setting, error) {
	settings.reads++
	return configport.Setting{Key: configport.WeComCorpID, Value: settings.value}, nil
}

func (*sidebarCorpSettings) Set(context.Context, configport.SetCommand) (configport.Setting, error) {
	return configport.Setting{}, configport.ErrUnknownSetting
}

func TestSidebarCorpReaderUsesIndependentConfiguredCorpID(t *testing.T) {
	settings := &sidebarCorpSettings{value: json.RawMessage(`"corp-admin"`)}
	reader := sidebarCorpReader{
		settings:              settings,
		fallback:              "corp-sidebar",
		fallbackAuthoritative: true,
	}

	corpID, err := reader.CorpID(context.Background())
	if err != nil {
		t.Fatalf("read independent Sidebar CorpID: %v", err)
	}
	if corpID != "corp-sidebar" {
		t.Fatalf("CorpID=%q want corp-sidebar", corpID)
	}
	if settings.reads != 0 {
		t.Fatalf("shared persistent CorpID reads=%d want=0", settings.reads)
	}
}
