package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestFinanceImportRequiresFrozenReferenceInputsBeforeDatabaseAccess(t *testing.T) {
	environment := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres://127.0.0.1:1/unreachable", ArchiveKey: strings.Repeat("a", 32)}
	for _, test := range []struct {
		name, key, run string
	}{
		{"missing run", strings.Repeat("k", 32), "0"},
		{"missing key", "", "2"},
		{"short key", "short", "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", test.key)
			err := run([]string{"--domain=finance", "--archive-run-id=frozen", "--dm01-run-id=" + test.run}, environment)
			if err == nil || !strings.Contains(err.Error(), "frozen DM01 source HMAC key") {
				t.Fatalf("expected reference precondition before connecting, got %v", err)
			}
		})
	}
}
