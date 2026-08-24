package main

import (
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"testing"
)

func TestRunRejectsUnsafeRuntimeConfiguration(t *testing.T) {
	runtime := appconfig.DM01Runtime{SourceDatabaseURL: "same", TargetDatabaseURL: "same", SourceHMACKey: "same", ArchiveKey: "same"}
	if err := run([]string{"--mode=full", "--source-manifest=x", "--manifest-sha256=x"}, runtime); err == nil {
		t.Fatal("same source/target accepted")
	}
}
