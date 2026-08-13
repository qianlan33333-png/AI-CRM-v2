package store

import (
	"context"
	"errors"
	"testing"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
)

func TestSyncStateRepositoryRequiresTransactionForAdvance(t *testing.T) {
	repository := NewSyncStateRepository(nil)
	if _, err := repository.LoadCursor(context.Background(), "external_contact_list:owner"); !errors.Is(err, wecomapp.ErrInvalidCursorSync) {
		t.Fatalf("LoadCursor() error = %v", err)
	}
	if err := repository.AdvanceCursor(context.Background(), "external_contact_list:owner", "", "next", false); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("AdvanceCursor() error = %v, want transaction requirement", err)
	}
}
