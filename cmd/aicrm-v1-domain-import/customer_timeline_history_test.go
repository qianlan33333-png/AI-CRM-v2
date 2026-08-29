package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestCustomerTimelineHistoryTerminalRetainsFieldHMAC(t *testing.T) {
	key := sha256.Sum256([]byte("timeline-source"))
	payload := sha256.Sum256([]byte("timeline-payload"))
	field := sha256.Sum256([]byte("timeline-field"))
	target := sha256.Sum256([]byte("timeline-target"))
	receipt := v1domain.TerminalReceipt{
		SourceKeyDigest: key, PayloadDigest: payload, Disposition: "import", TargetID: "19", TargetDigest: target,
		Metadata: customerTimelineHistoryMetadata(field),
	}
	terminal, found, err := customerTimelineHistoryTerminal("archive-run", receipt)
	if err != nil || !found || terminal.SourceKeyHMAC != key || terminal.PayloadHMAC != payload || terminal.FieldHMAC != field || terminal.TargetID != 19 || terminal.TargetDigest != target {
		t.Fatalf("field proof lost: terminal=%#v found=%t err=%v", terminal, found, err)
	}
	historyReceipt, found, err := customerTimelineHistoryReceipt(v1domain.SourceIdentifier(key), receipt)
	if err != nil || !found || historyReceipt.PayloadDigest != payload || historyReceipt.TargetDigest != target || historyReceipt.TargetID != 19 {
		t.Fatalf("writer receipt mismatch: %#v found=%t err=%v", historyReceipt, found, err)
	}

	for _, metadata := range []map[string]any{
		nil,
		{customerTimelineHistoryFieldMetadata: strings.ToUpper(hex.EncodeToString(field[:]))},
		{customerTimelineHistoryFieldMetadata: hex.EncodeToString(make([]byte, sha256.Size))},
		{customerTimelineHistoryFieldMetadata: hex.EncodeToString(field[:]), "unexpected": true},
	} {
		if _, err := customerTimelineHistoryFieldHMAC(metadata); err == nil {
			t.Fatalf("invalid field metadata accepted: %#v", metadata)
		}
	}
}

func TestCustomerTimelineHistoryCommandRejectsNonLocalInputsBeforeConnecting(t *testing.T) {
	if !validDomain(customerTimelineHistoryDomain) {
		t.Fatal("customer timeline history domain is not selectable")
	}
	for _, mode := range []string{"import", "reconcile"} {
		err := run([]string{"--domain=" + customerTimelineHistoryDomain, "--mode=" + mode, "--archive-run-id=archive", "--dm01-run-id=2"}, appconfig.V1ArchiveRuntime{
			TargetDatabaseURL: "postgres:///must-not-connect", SourceDatabaseURL: "postgres:///forbidden-v1",
			SourceHMACKey: strings.Repeat("h", sha256.Size), ArchiveKey: strings.Repeat("k", sha256.Size),
		})
		if err == nil || !strings.Contains(err.Error(), "local-only archive/DM01 keys") {
			t.Fatalf("%s accepted V1 source input or connected before validation: %v", mode, err)
		}
	}
}
