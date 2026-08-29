package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
)

func TestHXCMemberUsageHistoryRequiresLocalKeysBeforeConnecting(t *testing.T) {
	for _, mode := range []string{"import", "reconcile"} {
		for _, kind := range []string{"source", "hmac", "aes"} {
			env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres:///must-not-connect", ArchiveKey: strings.Repeat("k", 32), SourceHMACKey: strings.Repeat("h", 32)}
			switch kind {
			case "source":
				env.SourceDatabaseURL = "postgres:///forbidden-v1"
			case "hmac":
				env.SourceHMACKey = ""
			case "aes":
				env.ArchiveKey = ""
			}
			err := run([]string{"--domain=hxc-member-usage-history", "--mode=" + mode, "--archive-run-id=archive"}, env)
			if err == nil || (!strings.Contains(err.Error(), "local-only archive keys") && !strings.Contains(err.Error(), "32-byte archive key")) {
				t.Fatalf("%s/%s: %v", mode, kind, err)
			}
		}
	}
	if !validDomain("hxc-member-usage-history") {
		t.Fatal("domain not selectable")
	}
}

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

func TestHXCChatJobHistoryRequiresLocalArchiveKeys(t *testing.T) {
	if !validDomain("hxc-chat-job-history") {
		t.Fatal("history domain missing")
	}
	for _, mode := range []string{"import", "reconcile"} {
		for _, field := range []string{"source", "hmac", "archive"} {
			env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid://must-not-connect", SourceHMACKey: strings.Repeat("h", 32), ArchiveKey: strings.Repeat("k", 32)}
			switch field {
			case "source":
				env.SourceDatabaseURL = "postgres:///forbidden-source"
			case "hmac":
				env.SourceHMACKey = ""
			case "archive":
				env.ArchiveKey = ""
			}
			err := run([]string{"--domain=hxc-chat-job-history", "--mode=" + mode, "--archive-run-id=archive"}, env)
			if err == nil || (!strings.Contains(err.Error(), "local-only archive keys") && !strings.Contains(err.Error(), "32-byte archive key")) {
				t.Fatalf("%s/%s did not fail before DB configuration: %v", mode, field, err)
			}
		}
	}
}

func TestCycleObservationRequiresLocalArchiveKeysBeforeConnecting(t *testing.T) {
	if !validDomain("cycle-observation-history") {
		t.Fatal("cycle observation domain missing")
	}
	for _, mode := range []string{"import", "reconcile"} {
		for _, change := range []func(*appconfig.V1ArchiveRuntime){
			func(v *appconfig.V1ArchiveRuntime) { v.SourceDatabaseURL = "postgres:///never-read-v1" },
			func(v *appconfig.V1ArchiveRuntime) { v.SourceHMACKey = "" },
			func(v *appconfig.V1ArchiveRuntime) { v.ArchiveKey = "" },
		} {
			env := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "invalid-target-must-not-connect", SourceHMACKey: strings.Repeat("h", 32), ArchiveKey: strings.Repeat("k", 32)}
			change(&env)
			err := run([]string{"--domain=cycle-observation-history", "--mode=" + mode, "--archive-run-id=archive"}, env)
			if err == nil || (!strings.Contains(err.Error(), "local-only archive keys") && !strings.Contains(err.Error(), "32-byte archive key")) {
				t.Fatalf("guard failed before connection: %v", err)
			}
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
