package main

import (
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	"testing"
)

func TestRunRejectsUnsafeRuntimeConfiguration(t *testing.T) {
	runtime := appconfig.DM01Runtime{SourceDatabaseURL: "same", TargetDatabaseURL: "same", SourceHMACKey: "same", ArchiveKey: "same"}
	if err := run([]string{"--mode=full", "--source-manifest=x", "--manifest-sha256=x"}, runtime); err == nil {
		t.Fatal("same source/target accepted")
	}
}

func TestSourceIdentityRequiresManifestAndRuntimeAuthorization(t *testing.T) {
	identity := migration.DatabaseIdentity{ServerID: "1", Database: "legacy", Role: "dm01_reader", ReadOnly: true}
	manifest := migration.Manifest{SourceServerID: "1", SourceDatabase: "legacy", SourceReadRole: "dm01_reader"}
	if !sourceIdentityAllowed(identity, manifest, []string{identity.AllowlistValue()}) {
		t.Fatal("authorized read-only source rejected")
	}
	for name, candidate := range map[string]migration.DatabaseIdentity{
		"writable": {ServerID: "1", Database: "legacy", Role: "dm01_reader"},
		"database": {ServerID: "1", Database: "other", Role: "dm01_reader", ReadOnly: true},
		"role":     {ServerID: "1", Database: "legacy", Role: "other", ReadOnly: true},
	} {
		t.Run(name, func(t *testing.T) {
			if sourceIdentityAllowed(candidate, manifest, []string{candidate.AllowlistValue()}) {
				t.Fatal("unauthorized source accepted")
			}
		})
	}
	if sourceIdentityAllowed(identity, manifest, []string{identity.AllowlistValue(), identity.AllowlistValue()}) {
		t.Fatal("duplicate runtime allowlist accepted")
	}
}
