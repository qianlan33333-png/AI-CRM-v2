package main

import (
	"strings"
	"testing"
)

func TestParseConfigImport(t *testing.T) {
	env := map[string]string{
		"AICRM_WHITELIST_SOURCE_DATABASE_URL": "postgres:///frozen",
		"AICRM_DATABASE_URL":                  "postgres:///aicrm_v2_core",
		"AICRM_WHITELIST_SOURCE_DIGEST":       strings.Repeat("a", 64),
		"AICRM_WHITELIST_ARCHIVE_RUN_ID":      "v1-full-archive-20260827",
	}
	config, err := parseConfig([]string{"--mode=import", "--run-id=wli_release_20260830"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if config.mode != "import" || config.runID != "wli_release_20260830" || config.sourceURL == config.targetURL {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestParseConfigRejectsUnsafeRunIDAndDigest(t *testing.T) {
	env := func(key string) string {
		switch key {
		case "AICRM_WHITELIST_SOURCE_DATABASE_URL":
			return "postgres:///frozen"
		case "AICRM_DATABASE_URL":
			return "postgres:///aicrm_v2_core"
		case "AICRM_WHITELIST_SOURCE_DIGEST":
			return "not-a-digest"
		case "AICRM_WHITELIST_ARCHIVE_RUN_ID":
			return "v1-full-archive-20260827"
		default:
			return ""
		}
	}
	if _, err := parseConfig([]string{"--mode=import", "--run-id=../../bad"}, env); err == nil {
		t.Fatal("unsafe run id was accepted")
	}
	if _, err := parseConfig([]string{"--mode=import", "--run-id=wli_release_20260830"}, env); err == nil {
		t.Fatal("invalid digest was accepted")
	}
}
