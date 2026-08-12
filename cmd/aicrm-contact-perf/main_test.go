package main

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseOptionsRequiresSafeIsolatedDatabaseAndHardMinimums(t *testing.T) {
	valid := []string{
		"--database-url=postgres://postgres:secret@postgres:5432/aicrm_perf?sslmode=disable",
		"--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369",
	}
	opts, err := parseOptions(valid)
	if err != nil || opts.samples != requiredSamples || opts.warmups != requiredWarmups {
		t.Fatalf("parseOptions(valid) = %#v, %v", opts, err)
	}
	secret := "must-not-leak"
	invalid := [][]string{
		{},
		{"--database-url=postgres://postgres:" + secret + "@postgres:5432/aicrm?sslmode=disable", "--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369"},
		{"--database-url=postgres://postgres:" + secret + "@postgres:5432/aicrm_perf?sslmode=require", "--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369"},
		{"--database-url=postgres://postgres:" + secret + "@postgres:5432/aicrm_perf?sslmode=disable&application_name=x", "--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369"},
		{"--database-url=postgres://postgres:" + secret + "@postgres:5432/aicrm_perf?sslmode=disable", "--source-sha=ABC", "--samples=20"},
		{"--database-url=postgres://postgres:" + secret + "@postgres:5432/aicrm_perf?sslmode=disable", "--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369", "--samples=19"},
		{"--database-url=postgres://postgres:" + secret + "@postgres:5432/aicrm_perf?sslmode=disable", "--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369", "--warmups=2"},
	}
	for _, arguments := range invalid {
		_, parseErr := parseOptions(arguments)
		if parseErr == nil {
			t.Fatalf("parseOptions(%q) succeeded", arguments)
		}
		if strings.Contains(parseErr.Error(), secret) {
			t.Fatal("parse error leaked the database password")
		}
	}
}

func TestQueryMatrixCoversEveryFilterTimePageAndLimitCombination(t *testing.T) {
	seen := map[string]bool{}
	for _, item := range scenarios() {
		query := queryForScenario(item)
		key := strings.Join([]string{
			strconv.Itoa(item.selectorMask), boolText(item.deleted), item.addedMode.String(),
			item.interactMode.String(), pageName(item.nextPage), strconv.Itoa(int(item.limit)),
		}, "/")
		if seen[key] {
			t.Fatalf("duplicate query matrix key %s", key)
		}
		seen[key] = true
		if query.Watermark != benchmarkWatermark || query.Limit != item.limit || query.IsDeleted != item.deleted ||
			(query.AddedAfter == nil && (item.addedMode == timeAfter || item.addedMode == timeClosed)) ||
			(query.AddedBefore == nil && (item.addedMode == timeBefore || item.addedMode == timeClosed)) ||
			(query.LastInteractAfter == nil && (item.interactMode == timeAfter || item.interactMode == timeClosed)) ||
			(query.LastInteractBefore == nil && (item.interactMode == timeBefore || item.interactMode == timeClosed)) {
			t.Fatalf("invalid query for scenario %#v: %#v", item, query)
		}
	}
	if len(seen) != 4096 {
		t.Fatalf("matrix size = %d, want 4096", len(seen))
	}
}

func TestPercentile95UsesNearestRank(t *testing.T) {
	values := make([]time.Duration, 20)
	for index := range values {
		values[index] = time.Duration(index+1) * time.Millisecond
	}
	if got := percentile95(values); got != 19*time.Millisecond {
		t.Fatalf("percentile95() = %v, want 19ms", got)
	}
	if got := maxDuration(values); got != 20*time.Millisecond {
		t.Fatalf("maxDuration() = %v, want 20ms", got)
	}
}

func TestWalkPlanRejectsOnlyTargetSequentialScansAndCollectsEvidence(t *testing.T) {
	raw := `{"Node Type":"Nested Loop","Shared Hit Blocks":2,"Plans":[{"Node Type":"Seq Scan","Relation Name":"customers","Shared Read Blocks":3},{"Node Type":"Seq Scan","Relation Name":"unrelated"},{"Node Type":"Index Only Scan","Relation Name":"customer_tags","Shared Hit Blocks":5}]}`
	var plan map[string]any
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		t.Fatal(err)
	}
	evidence := planEvidence{}
	walkPlan(plan, &evidence)
	sortEvidence(&evidence)
	if !reflect.DeepEqual(evidence.ForbiddenScans, []string{"customers"}) {
		t.Fatalf("forbidden scans = %v", evidence.ForbiddenScans)
	}
	if evidence.SharedHit != 7 || evidence.SharedRead != 3 {
		t.Fatalf("buffers = hit %d read %d", evidence.SharedHit, evidence.SharedRead)
	}
}

func TestLinuxMemoryEvidenceRequiresMemoryAndActiveSwap(t *testing.T) {
	path := t.TempDir() + "/meminfo"
	if err := osWriteFile(path, []byte("MemTotal:       4023000 kB\nSwapTotal:      4194304 kB\n")); err != nil {
		t.Fatal(err)
	}
	if memory, swap, err := linuxMemoryEvidence(path); err != nil || memory != 4_023_000 || swap != 4_194_304 {
		t.Fatalf("linuxMemoryEvidence() = %d, %d, %v", memory, swap, err)
	}
	if err := osWriteFile(path, []byte("MemTotal: secret bytes\n")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := linuxMemoryEvidence(path); err == nil {
		t.Fatal("malformed memory evidence was accepted")
	}
}

func boolText(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func sortEvidence(evidence *planEvidence) {
	sort.Strings(evidence.NodeTypes)
	sort.Strings(evidence.ForbiddenScans)
	evidence.NodeTypes = uniqueStrings(evidence.NodeTypes)
	evidence.ForbiddenScans = uniqueStrings(evidence.ForbiddenScans)
}

func osWriteFile(path string, contents []byte) error {
	return os.WriteFile(path, contents, 0o600)
}
