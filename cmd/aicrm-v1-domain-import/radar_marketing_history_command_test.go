package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestRadarMarketingHistoryCommandRoutes(t *testing.T) {
	for _, domain := range []string{"radar-click-history", "marketing-config-history"} {
		if !validDomain(domain) {
			t.Fatal("history domain missing")
		}
		err := run([]string{"--mode=reconcile", "--domain=" + domain, "--archive-run-id=archive"}, appconfig.V1ArchiveRuntime{TargetDatabaseURL: "://invalid"})
		if err == nil || strings.Contains(err.Error(), "reconcile requires") {
			t.Fatalf("reconcile not routed: %v", err)
		}
	}
	err := run([]string{"--mode=import", "--domain=marketing-config-history", "--archive-run-id=archive"}, appconfig.V1ArchiveRuntime{ArchiveKey: strings.Repeat("x", 32), TargetDatabaseURL: "://invalid"})
	if err == nil || strings.Contains(err.Error(), "migration-actor") || strings.Contains(err.Error(), "dm01-run-id") {
		t.Fatalf("marketing history acquired unrelated dependency: %v", err)
	}
}
