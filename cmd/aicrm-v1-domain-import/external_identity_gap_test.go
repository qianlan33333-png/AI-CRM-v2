package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestExternalIdentityGapCLIRequiresLocalArchiveKeysBeforeConnecting(t *testing.T) {
	if !validDomain("external-identity-gap") || validDomain("external-identity-gap-all") {
		t.Fatal("identity gap domain is not closed")
	}
	for _, mode := range []string{"import", "reconcile"} {
		env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid-target", ArchiveKey: strings.Repeat("k", 32)}
		err := run([]string{"--domain=external-identity-gap", "--mode=" + mode, "--archive-run-id=test", "--dm01-run-id=2"}, env)
		if err == nil || !strings.Contains(err.Error(), "local-only archive/DM01 keys") {
			t.Fatalf("%s should reject incomplete local keys before connecting: %v", mode, err)
		}
	}
}
