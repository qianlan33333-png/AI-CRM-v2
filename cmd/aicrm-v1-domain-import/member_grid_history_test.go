package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestMemberGridHistoryProductReferenceRequiresActualTypedTarget(t *testing.T) {
	at := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	value := productport.ServicePeriodHistoryDefinition{ID: 7, SourceDefinitionID: 41, ProductID: 9, MembershipConfigID: "old", CreatedAt: at, UpdatedAt: at}
	receipt := v1domain.TerminalReceipt{TargetID: strconv.FormatInt(value.ID, 10), TargetDigest: productapp.ServicePeriodHistoryDefinitionTargetDigest(value)}
	if !memberGridHistoryProductMatches(value, 41, receipt) {
		t.Fatal("valid Product crosswalk rejected")
	}
	for _, mutate := range []func(*productport.ServicePeriodHistoryDefinition){
		func(v *productport.ServicePeriodHistoryDefinition) { v.ID++ },
		func(v *productport.ServicePeriodHistoryDefinition) { v.SourceDefinitionID++ },
		func(v *productport.ServicePeriodHistoryDefinition) { v.ProductID = 0 },
		func(v *productport.ServicePeriodHistoryDefinition) { v.ProductID++ },
		func(v *productport.ServicePeriodHistoryDefinition) { v.MembershipConfigID = "drift" },
	} {
		changed := value
		mutate(&changed)
		if memberGridHistoryProductMatches(changed, 41, receipt) {
			t.Fatal("source or target drift accepted")
		}
	}
}

func TestMemberGridHistoryRecoveryFileIsNarrowAndPreservesFalse(t *testing.T) {
	entry := v1membergridhistory.UsageSnapshotRecoveryEntry{Scope: v1membergridhistory.FixedUsageSnapshotRecoveryScope(), SourceKeyHMAC: [32]byte{1}, OriginalPayloadHMAC: [32]byte{2}, OriginalFieldHMAC: [32]byte{3}, EntryHMAC: [32]byte{4}, HasTokenUsage: false}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "recovery.jsonl")
	header, err := json.Marshal(map[string]any{"scope": entry.Scope, "query_sha256": strings.Repeat("1", 64), "row_count": 1})
	if err != nil {
		t.Fatal(err)
	}
	write := func(data []byte) {
		t.Helper()
		content := append(append(append([]byte(nil), header...), '\n'), data...)
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(append(data, '\n'))
	entries, err := loadMemberGridHistoryRecovery(path, entry.Scope.ArchiveRunID)
	if err != nil || len(entries) != 1 || entries[0].HasTokenUsage || entries[0] != entry {
		t.Fatal("false recovery entry changed")
	}
	for _, content := range [][]byte{nil, []byte("not-json"), append(append(append([]byte(nil), data...), '\n'), data...), append(append([]byte(nil), data[:len(data)-1]...), []byte(`,"unexpected":true}`)...)} {
		write(content)
		if _, err := loadMemberGridHistoryRecovery(path, entry.Scope.ArchiveRunID); err == nil {
			t.Fatal("invalid/duplicate/unknown recovery file accepted")
		}
	}
	write(data)
	originalHeader := append([]byte(nil), header...)
	header = []byte(`{"scope":{},"query_sha256":"bad","row_count":1}`)
	write(data)
	if _, err := loadMemberGridHistoryRecovery(path, entry.Scope.ArchiveRunID); err == nil {
		t.Fatal("invalid recovery header accepted")
	}
	header = originalHeader
	write(data)
	if _, err := loadMemberGridHistoryRecovery(path, "another-run"); err == nil {
		t.Fatal("wrong frozen run accepted")
	}
	entry.Scope.Field = "another-field"
	data, _ = json.Marshal(entry)
	write(data)
	if _, err := loadMemberGridHistoryRecovery(path, v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID); err == nil {
		t.Fatal("other field recovery accepted")
	}
}

func TestMemberGridHistoryImportPrerequisitesFailBeforeConnecting(t *testing.T) {
	args := []string{"--domain=member-grid-history", "--archive-run-id=v1-full-archive-20260827"}
	env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid-target-url", ArchiveKey: strings.Repeat("a", 32)}
	t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", "")
	if err := run(args, env); err == nil || !strings.Contains(err.Error(), "dm01-run-id") {
		t.Fatal("missing DM01 must fail before connection")
	}
	t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", strings.Repeat("d", 32))
	args = append(args, "--dm01-run-id=2")
	if err := run(args, env); err == nil || !strings.Contains(err.Error(), "archive source HMAC key") {
		t.Fatal("missing source HMAC must fail before connection")
	}
	env.SourceHMACKey = strings.Repeat("s", 32)
	if err := run(args, env); err == nil || !strings.Contains(err.Error(), "recovery file") {
		t.Fatal("missing recovery must fail before connection")
	}
	if !validDomain("member-grid-history") {
		t.Fatal("domain flag missing")
	}
}
