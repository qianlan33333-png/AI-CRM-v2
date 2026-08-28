package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestGroupOpsImportRequiresHistoricalInputsBeforeDatabaseAccess(t *testing.T) {
	if !validDomain("groupops") {
		t.Fatal("groupops must be an explicit import domain")
	}
	environment := appconfig.V1ArchiveRuntime{
		TargetDatabaseURL: "postgres://127.0.0.1:1/must-not-connect",
		ArchiveKey:        strings.Repeat("a", 32),
	}

	t.Run("missing migration actor", func(t *testing.T) {
		t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", strings.Repeat("k", 32))
		err := run([]string{"--domain=groupops", "--archive-run-id=frozen", "--dm01-run-id=2"}, environment)
		if err == nil || !strings.Contains(err.Error(), "migration-actor") {
			t.Fatalf("expected actor precondition before connecting, got %v", err)
		}
	})

	for _, test := range []struct {
		name, key, run string
	}{
		{"missing DM01 run", strings.Repeat("k", 32), "0"},
		{"missing DM01 key", "", "2"},
		{"short DM01 key", "short", "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", test.key)
			err := run([]string{"--domain=groupops", "--archive-run-id=frozen", "--migration-actor=1", "--dm01-run-id=" + test.run}, environment)
			if err == nil || !strings.Contains(err.Error(), "frozen DM01 source HMAC key") {
				t.Fatalf("expected DM01 precondition before connecting, got %v", err)
			}
		})
	}
}
