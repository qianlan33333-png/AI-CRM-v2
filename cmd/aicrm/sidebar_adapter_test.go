package main

import (
	"context"
	"encoding/json"
	"testing"

	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

type sidebarCorpSettings struct {
	value json.RawMessage
	reads int
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
