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

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

const testSourceSHA = "33f6e19792a6d44686642236fb99d6a4e76c3369"
const testMainCIURL = "https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31569336280"

func TestParseOptionsRequiresSafeIsolatedDatabaseAndHardMinimums(t *testing.T) {
	root := t.TempDir()
	path := root + "/database-url"
	value := "postgres://postgres:secret@127.0.0.1:5432/aicrm_perf?sslmode=disable"
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := []string{
		"--database-url-file=" + path,
		"--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369",
		"--main-ci-url=" + testMainCIURL,
	}
	opts, err := parseOptions(valid)
	if err != nil || opts.samples != requiredSamples || opts.warmups != requiredWarmups {
		t.Fatalf("parseOptions(valid) = %#v, %v", opts, err)
	}
	secret := "must-not-leak"
	invalid := [][]string{
		{},
		{"--database-url-file=" + path, "--source-sha=" + testSourceSHA},
		{"--database-url=" + value, "--source-sha=33f6e19792a6d44686642236fb99d6a4e76c3369"},
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

func TestParseOptionsAcceptsOnlyExclusiveReceiptVerificationMode(t *testing.T) {
	opts, err := parseOptions([]string{"--verify-receipt=/tmp/p3-c06.json", "--source-sha=" + testSourceSHA, "--main-ci-url=" + testMainCIURL})
	if err != nil || opts.receiptPath != "/tmp/p3-c06.json" {
		t.Fatalf("parse receipt mode = %#v, %v", opts, err)
	}
	for _, arguments := range [][]string{
		{"--verify-receipt=/tmp/p3-c06.json"},
		{"--verify-receipt=/tmp/p3-c06.json", "--source-sha=" + testSourceSHA},
		{"--verify-receipt=/tmp/p3-c06.json", "--source-sha=" + testSourceSHA, "--main-ci-url=" + testMainCIURL, "--samples=21"},
		{"--verify-receipt=/tmp/p3-c06.json", "--database-url=postgres://u:p@postgres/aicrm_perf?sslmode=disable"},
	} {
		if _, parseErr := parseOptions(arguments); parseErr == nil {
			t.Fatalf("mixed receipt arguments %q were accepted", arguments)
		}
	}
}

func TestDatabaseURLFileMustBeAbsolutePrivateRegularAndSingleLine(t *testing.T) {
	root := t.TempDir()
	valid := root + "/database-url"
	value := "postgres://postgres:secret@127.0.0.1:5432/aicrm_perf?sslmode=disable"
	if err := os.WriteFile(valid, []byte(value+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := databaseURLFromFile(valid); err != nil || got != value {
		t.Fatalf("databaseURLFromFile(valid) = %q, %v", got, err)
	}
	link := root + "/link"
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", link} {
		if _, err := databaseURLFromFile(path); err == nil {
			t.Fatalf("unsafe path %q was accepted", path)
		}
	}
	world := root + "/world"
	if err := os.WriteFile(world, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseURLFromFile(world); err == nil {
		t.Fatal("world-readable database URL file was accepted")
	}
	multiline := root + "/multiline"
	if err := os.WriteFile(multiline, []byte(value+"\n"+value), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseURLFromFile(multiline); err == nil {
		t.Fatal("multi-line database URL file was accepted")
	}
}

func TestParseOptionsRequiresDatabaseURLFileOnly(t *testing.T) {
	root := t.TempDir()
	path := root + "/database-url"
	value := "postgres://postgres:secret@127.0.0.1:5432/aicrm_perf?sslmode=disable"
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	sha := "33f6e19792a6d44686642236fb99d6a4e76c3369"
	opts, err := parseOptions([]string{"--database-url-file=" + path, "--source-sha=" + sha, "--main-ci-url=" + testMainCIURL})
	if err != nil || opts.databaseURL != value {
		t.Fatalf("parse file URL mode = %#v, %v", opts, err)
	}
	if _, err := parseOptions([]string{"--database-url=" + value, "--source-sha=" + sha}); err == nil {
		t.Fatal("database URL in process arguments was accepted")
	}
}

func TestValidateDatabaseURLMatchesGeneratorLoopbackContract(t *testing.T) {
	for _, value := range []string{
		"postgres://synthetic@127.0.0.1/aicrm_perf?sslmode=disable",
		"postgresql://synthetic@[::1]:5432/aicrm_perf?sslmode=disable",
	} {
		if err := validateDatabaseURL(value); err != nil {
			t.Fatalf("validateDatabaseURL(%q) = %v", value, err)
		}
	}
	for _, value := range []string{
		"postgres://synthetic@localhost/aicrm_perf?sslmode=disable",
		"postgres://synthetic@prod:5432/aicrm_perf?sslmode=disable",
		"postgres://synthetic@150.158.82.186/aicrm_perf?sslmode=disable",
		"postgres://synthetic@127.0.0.1/aicrm?sslmode=disable",
		"postgres://synthetic@127.0.0.1/aicrm_perf?sslmode=require",
	} {
		if err := validateDatabaseURL(value); err == nil {
			t.Fatalf("validateDatabaseURL(%q) succeeded", value)
		}
	}
}

func TestTrustedMainCIURLRequiresExactRepositoryActionsRun(t *testing.T) {
	if !isTrustedMainCIURL(testMainCIURL) {
		t.Fatal("expected main CI URL was rejected")
	}
	for _, value := range []string{
		"",
		"http://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/1",
		"https://github.com/other/AI-CRM-v2/actions/runs/1",
		"https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/0",
		"https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/1?attempt=2",
	} {
		if isTrustedMainCIURL(value) {
			t.Fatalf("unsafe main CI URL %q was accepted", value)
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
			(query.Keyword == "kw017") != (item.selectorMask&1 != 0) ||
			(query.OwnerStaffID != nil) != (item.selectorMask&2 != 0) ||
			(query.StageID != nil) != (item.selectorMask&4 != 0) ||
			(query.ChannelID != nil) != (item.selectorMask&8 != 0) ||
			(query.TagID != nil) != (item.selectorMask&16 != 0) ||
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

func TestValidateScenarioPageRequiresFullContinuationWithoutOverlap(t *testing.T) {
	item := scenario{nextPage: true, limit: 2}
	anchor := contactapp.CustomerListStoreResult{
		Items: []contactapp.CustomerRecord{{ID: 4}, {ID: 3}}, HasMore: true,
	}
	valid := contactapp.CustomerListStoreResult{
		Items: []contactapp.CustomerRecord{{ID: 2}, {ID: 1}}, HasMore: true,
	}
	if err := validateScenarioPage(item, anchor, valid); err != nil {
		t.Fatalf("validateScenarioPage(valid) = %v", err)
	}
	for _, invalid := range []contactapp.CustomerListStoreResult{
		{Items: []contactapp.CustomerRecord{{ID: 2}}, HasMore: true},
		{Items: []contactapp.CustomerRecord{{ID: 2}, {ID: 1}}, HasMore: false},
		{Items: []contactapp.CustomerRecord{{ID: 3}, {ID: 2}}, HasMore: true},
	} {
		if err := validateScenarioPage(item, anchor, invalid); err == nil {
			t.Fatalf("validateScenarioPage(%#v) succeeded", invalid)
		}
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

func TestValidateReceiptRequiresEveryFastPlanSafeScenario(t *testing.T) {
	valid := validReceipt()
	if err := validateReceipt(valid, testSourceSHA, testMainCIURL); err != nil {
		t.Fatalf("validateReceipt(valid) error = %v", err)
	}

	mutations := []struct {
		name   string
		mutate func(*report)
	}{
		{name: "missing case", mutate: func(value *report) { value.Cases = value.Cases[:len(value.Cases)-1] }},
		{name: "slow p95", mutate: func(value *report) { value.Cases[0].P95MS = 200 }},
		{name: "sequential scan", mutate: func(value *report) { value.Cases[0].Plans[0].ForbiddenScans = []string{"customers"} }},
		{name: "missing samples", mutate: func(value *report) { value.Cases[0].Samples = 19 }},
		{name: "wrong source", mutate: func(value *report) { value.Environment.BinaryVCSRevision = strings.Repeat("a", 40) }},
		{name: "unexpected source", mutate: func(value *report) {
			value.Environment.SourceSHA = strings.Repeat("a", 40)
			value.Environment.BinaryVCSRevision = strings.Repeat("a", 40)
		}},
		{name: "unexpected main CI", mutate: func(value *report) {
			value.Environment.MainCIURL = "https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/1"
		}},
		{name: "missing time distribution", mutate: func(value *report) { value.Dataset.AddedBefore = 0 }},
		{name: "negative buffers", mutate: func(value *report) { value.Cases[0].Plans[0].SharedRead = -1 }},
		{name: "inconsistent raw plan", mutate: func(value *report) { value.Cases[0].Plans[0].ExecutionMS = 2 }},
		{name: "fake environment", mutate: func(value *report) { value.EvidenceClass = "local" }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			candidate := validReceipt()
			test.mutate(&candidate)
			if err := validateReceipt(candidate, testSourceSHA, testMainCIURL); err == nil {
				t.Fatal("invalid receipt was accepted")
			}
		})
	}
}

func TestVerifyReceiptFileIsStrictAndRejectsSymlink(t *testing.T) {
	receipt := validReceipt()
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := root + "/receipt.json"
	if err := osWriteFile(path, encoded); err != nil {
		t.Fatal(err)
	}
	if err := verifyReceiptFile(path, testSourceSHA, testMainCIURL); err != nil {
		t.Fatalf("verifyReceiptFile(valid) error = %v", err)
	}
	if err := osWriteFile(path, append(encoded, []byte(` {}`)...)); err != nil {
		t.Fatal(err)
	}
	if err := verifyReceiptFile(path, testSourceSHA, testMainCIURL); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	if err := osWriteFile(path, encoded); err != nil {
		t.Fatal(err)
	}
	link := root + "/receipt-link.json"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := verifyReceiptFile(link, testSourceSHA, testMainCIURL); err == nil {
		t.Fatal("symlink receipt was accepted")
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

func validReceipt() report {
	const sha = testSourceSHA
	result := report{
		Kind: "contact_customer_list_s_tier_hard_gate", EvidenceClass: "authorized_test_server_synthetic",
		GeneratedAt: "2026-08-12T00:00:00Z", ThresholdMS: 200, Passed: true,
		Environment: environmentEvidence{
			SourceSHA: sha, MainCIURL: testMainCIURL, BinaryVCSRevision: sha, Database: requiredDatabase, PostgreSQLVersion: "160014",
			CPUs: 2, MemoryKiB: 4_000_000, SwapKiB: 4_194_304, GoMemoryLimitBytes: 768 * 1024 * 1024,
			SharedBuffers: "1GB", EffectiveCacheSize: "2GB", WorkMem: "8MB", MaxConnections: "40",
		},
		Dataset: datasetEvidence{
			Customers: requiredCustomers, CustomerTags: requiredTags, Staff: 64, Stages: 8, Channels: 12,
			Tags: 50, Deleted: requiredCustomers / 20, HotActive: 500, HotDeleted: 500,
			AddedBefore: 50_000, AddedWithin: 100_000, AddedAfter: 50_000,
			InteractBefore: 50_000, InteractWithin: 100_000, InteractAfter: 50_000,
		},
		CombinationCount: 4096, SampleCount: 4096 * requiredSamples,
		GlobalP50MS: 3, GlobalP95MS: 4, GlobalMaxMS: 5,
	}
	for _, item := range scenarios() {
		result.Cases = append(result.Cases, caseEvidence{
			ID: scenarioID(item), SelectorMask: item.selectorMask, Deleted: item.deleted,
			AddedMode: item.addedMode.String(), InteractMode: item.interactMode.String(), Page: pageName(item.nextPage),
			Limit: item.limit, Samples: requiredSamples, P50MS: 3, P95MS: 4, MaxMS: 5,
			Matched: int(item.limit), HasMore: true,
			Plans: []planEvidence{
				{Query: "ListCustomerIDsBounded", ExecutionMS: 1, PlanningMS: 0.1, NodeTypes: []string{"Index Only Scan"}, ForbiddenScans: []string{}, Explain: rawPlan("Index Only Scan")},
				{Query: "ListCustomers", ExecutionMS: 1, PlanningMS: 0.1, NodeTypes: []string{"Index Scan"}, ForbiddenScans: []string{}, Explain: rawPlan("Index Scan")},
			},
		})
	}
	result.SlowestCase = result.Cases[0].ID
	return result
}

func rawPlan(nodeType string) json.RawMessage {
	return json.RawMessage(`[{"Plan":{"Node Type":"` + nodeType + `","Shared Hit Blocks":0,"Shared Read Blocks":0},"Planning Time":0.1,"Execution Time":1}]`)
}
