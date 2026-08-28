package main

import (
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"strings"
	"testing"
)

func TestWeComContactHistoryCommandHasNoDM01OrActorBinding(t *testing.T) {
	if !validDomain(weComContactHistoryDomain) || weComContactHistoryImportVersion == domainImportVersion {
		t.Fatal("history command scope is not independent")
	}
	for _, mode := range []string{"import", "reconcile"} {
		// Invalid URL is rejected by parsing, without making a database connection.
		err := run([]string{"--domain=" + weComContactHistoryDomain, "--mode=" + mode, "--archive-run-id=v1-full-archive-20260827"}, appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres://%", ArchiveKey: strings.Repeat("k", 32)})
		if err == nil || strings.Contains(err.Error(), "dm01-run-id") || strings.Contains(err.Error(), "migration-actor") || strings.Contains(err.Error(), "reconcile requires") {
			t.Fatalf("unexpected command scope: %v", err)
		}
	}
}
