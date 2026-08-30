package main

import (
	"strings"
	"testing"
	"time"
)

func TestCleanupInventoryRejectsBroadAndProtectedTargets(t *testing.T) {
	for _, item := range []inventoryItem{
		{Type: "path", Name: "/", Reason: "old V2"},
		{Type: "path", Name: "/srv/aicrm/v1/archive", Reason: "old V2"},
		{Type: "database", Name: "aicrm_v2_core", Reason: "old V2"},
		{Type: "volume", Name: "aicrm_*", Reason: "old V2"},
		{Type: "container", Name: "hxc-current", Reason: "old V2"},
		{Type: "container", Name: "aicrm-v1-old", Reason: "old V2"},
		{Type: "path", Name: "/srv/aa-old/runtime", Reason: "old V2"},
	} {
		if err := validateInventoryItem(item); err == nil {
			t.Fatalf("unsafe item was accepted: %#v", item)
		}
	}
	if err := validateInventoryItem(inventoryItem{Type: "path", Name: "/srv/aicrm-v2/legacy-runtime", Reason: "replaced old V2 runtime"}); err != nil {
		t.Fatalf("exact old V2 path was rejected: %v", err)
	}
}

func TestCleanupManifestDigestDetectsMutation(t *testing.T) {
	manifest := planManifest{Version: 1, TargetDatabase: "aicrm_v2_core", GeneratedAt: time.Unix(1, 0).UTC(), Items: []planItem{{Type: "database", Name: "aicrm", SizeBytes: 7, Reason: "old V2"}}}
	digest, err := manifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ContentSHA256 = digest
	verified, err := manifestDigest(manifest)
	if err != nil || verified != digest {
		t.Fatalf("manifest did not verify: %s %v", verified, err)
	}
	manifest.Items[0].SizeBytes++
	mutated, _ := manifestDigest(manifest)
	if mutated == digest {
		t.Fatal("manifest mutation did not change digest")
	}
}

func TestCleanupConfigRequiresExactlyOneModeAndBothGates(t *testing.T) {
	env := func(string) string { return "" }
	if _, err := parseConfig([]string{"--plan", "--apply", "--manifest=/tmp/plan.json"}, env); err == nil {
		t.Fatal("two modes were accepted")
	}
	if _, err := parseConfig([]string{"--apply", "--manifest=/tmp/plan.json"}, env); err == nil {
		t.Fatal("apply without gates was accepted")
	}
}

func TestGateReceiptValidation(t *testing.T) {
	receipt := gateReceipt{Status: "passed", TargetDatabase: "aicrm_v2_core", SourceDigest: strings.Repeat("a", 64), ReleaseSHA: strings.Repeat("b", 40)}
	if !isSHA256(receipt.SourceDigest) || !validReleaseSHA(receipt.ReleaseSHA) {
		t.Fatal("valid gate hashes were rejected")
	}
}
