package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestParseActorIDs(t *testing.T) {
	actors, err := parseActorIDs("QianLan=1,ZhaoYanFang=2")
	if err != nil || actors["QianLan"] != 1 || actors["ZhaoYanFang"] != 2 {
		t.Fatalf("actors/error = %#v/%v", actors, err)
	}
	for _, invalid := range []string{"", "QianLan", "QianLan=0", "QianLan=1,QianLan=2", " QianLan=1"} {
		if _, err := parseActorIDs(invalid); err == nil {
			t.Fatalf("%q unexpectedly accepted", invalid)
		}
	}
}

func TestStaticAndChannelPackagesRequireExplicitDM01BeforeConnecting(t *testing.T) {
	t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", "")
	for _, domain := range []string{"static", "channel"} {
		err := run([]string{"--domain=" + domain, "--archive-run-id=archive", "--migration-actor=1"}, appconfig.V1ArchiveRuntime{
			TargetDatabaseURL: "postgres:///must-not-connect", ArchiveKey: strings.Repeat("k", 32),
		})
		if err == nil || !strings.Contains(err.Error(), "dm01-run-id") {
			t.Fatalf("missing DM01 binding must fail before database access: %v", err)
		}
		if !validDomain(domain) {
			t.Fatal("package must be explicitly selectable")
		}
	}
	if staticImportVersion == domainImportVersion || channelImportVersion == staticImportVersion || channelImportVersion == domainImportVersion {
		t.Fatal("packages must use separate immutable import versions")
	}
}
