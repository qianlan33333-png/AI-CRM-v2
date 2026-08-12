package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testPerformanceURL = "postgres://synthetic:private-password@127.0.0.1:5432/aicrm_perf?sslmode=disable"

func TestParseArgumentsAcceptsOnlyFrozenContract(t *testing.T) {
	path := writeDatabaseURLFile(t, testPerformanceURL, 0o600)
	config, err := parseArguments([]string{
		"--seed=" + seedText,
		"--database-url-file=" + path,
		"--reset-token=" + resetToken,
	})
	if err != nil {
		t.Fatalf("parseArguments() error = %v", err)
	}
	if config.databaseURL != testPerformanceURL || config.seed != datasetSeed {
		t.Fatalf("config = %#v", config)
	}

	for _, args := range [][]string{
		nil,
		{"--database-url=" + testPerformanceURL, "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + path, "--reset-token=" + resetToken},
		{"--database-url-file=" + path, "--reset-token=wrong", "--seed=" + seedText},
		{"--database-url-file=" + path, "--reset-token=" + resetToken, "--seed=1"},
		{"--database-url-file=" + path, "--reset-token=" + resetToken, "--seed=" + seedText, "--count=1"},
		{"--database-url-file=" + path, "--reset-token=" + resetToken, "--seed=" + seedText, "--overwrite=true"},
	} {
		if _, err := parseArguments(args); !errorsIsInvalidArguments(err) {
			t.Fatalf("parseArguments(%q) error = %v, want invalid arguments", args, err)
		}
	}
	if customerCount != 200000 || tagsPerCustomer != 3 || deletedCount != 10000 || hotCohortPerState != 500 {
		t.Fatalf("fixed dataset contract drifted: customers=%d tags=%d deleted=%d hot=%d", customerCount, tagsPerCustomer, deletedCount, hotCohortPerState)
	}
}

func TestParseArgumentsReadsSecureDatabaseURLFile(t *testing.T) {
	path := writeDatabaseURLFile(t, testPerformanceURL+"\n", 0o600)
	config, err := parseArguments([]string{
		"--database-url-file=" + path,
		"--reset-token=" + resetToken,
		"--seed=" + seedText,
	})
	if err != nil {
		t.Fatalf("parseArguments() error = %v", err)
	}
	if config.databaseURL != testPerformanceURL || config.seed != datasetSeed {
		t.Fatalf("config = %#v", config)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := parseArguments([]string{
		"--database-url-file=" + path,
		"--reset-token=" + resetToken,
		"--seed=" + seedText,
	}); err != nil {
		t.Fatalf("parseArguments() for 0400 file error = %v", err)
	}
}

func TestParseArgumentsRejectsUnsafeDatabaseURLFileInputs(t *testing.T) {
	securePath := writeDatabaseURLFile(t, testPerformanceURL, 0o600)
	worldReadablePath := writeDatabaseURLFile(t, testPerformanceURL, 0o644)
	groupWritablePath := writeDatabaseURLFile(t, testPerformanceURL, 0o620)
	multiLinePath := writeDatabaseURLFile(t, testPerformanceURL+"\npostgres://other", 0o600)
	overlongPath := writeDatabaseURLFile(t, strings.Repeat("x", 4097), 0o600)
	linkPath := filepath.Join(t.TempDir(), "database-url-link")
	if err := os.Symlink(securePath, linkPath); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"--database-url-file=relative.txt", "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + worldReadablePath, "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + groupWritablePath, "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + multiLinePath, "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + overlongPath, "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + linkPath, "--reset-token=" + resetToken, "--seed=" + seedText},
		{
			"--database-url=" + testPerformanceURL,
			"--database-url-file=" + securePath,
			"--reset-token=" + resetToken,
			"--seed=" + seedText,
		},
	} {
		if _, err := parseArguments(args); !errorsIsInvalidArguments(err) {
			t.Fatalf("parseArguments(%q) error = %v, want invalid arguments", args, err)
		}
	}
}

func TestValidateDatabaseURLRejectsAnythingButIsolatedPerfDatabase(t *testing.T) {
	for _, databaseURL := range []string{
		"postgres://synthetic@127.0.0.1/aicrm_perf?sslmode=disable",
		"postgresql://synthetic@[::1]:5432/aicrm_perf?sslmode=disable",
	} {
		if _, err := validateDatabaseURL(databaseURL); err != nil {
			t.Fatalf("validateDatabaseURL(%q) error = %v", databaseURL, err)
		}
	}

	for _, databaseURL := range []string{
		"postgres://synthetic@127.0.0.1/aicrm?sslmode=disable",
		"postgres://synthetic@127.0.0.1/aicrm_perf?sslmode=require",
		"postgres://synthetic@127.0.0.1/aicrm_perf?application_name=perf&sslmode=disable",
		"postgres://synthetic@127.0.0.1/aicrm_perf?sslmode=disable#fragment",
		"postgres://synthetic@150.158.82.186/aicrm_perf?sslmode=disable",
		"postgres://synthetic@localhost/aicrm_perf?sslmode=disable",
		"postgres://synthetic@prod:5432/aicrm_perf?sslmode=disable",
		"postgres://synthetic@staging:5432/aicrm_perf?sslmode=disable",
		"mysql://synthetic@127.0.0.1/aicrm_perf?sslmode=disable",
		"postgres:///aicrm_perf?sslmode=disable",
	} {
		if _, err := validateDatabaseURL(databaseURL); !errorsIsInvalidArguments(err) {
			t.Fatalf("validateDatabaseURL(%q) error = %v, want invalid arguments", databaseURL, err)
		}
	}
}

func TestForbiddenCustomerColumnsRejectExternalIdentityShapes(t *testing.T) {
	for _, columnName := range []string{"phone", "mobile_number", "union_id", "open_id", "external_contact_id"} {
		if !isForbiddenCustomerColumn(columnName) {
			t.Fatalf("isForbiddenCustomerColumn(%q) = false", columnName)
		}
	}
	for _, columnName := range []string{"id", "name", "channel_id", "updated_at"} {
		if isForbiddenCustomerColumn(columnName) {
			t.Fatalf("isForbiddenCustomerColumn(%q) = true", columnName)
		}
	}
}

func TestRunDoesNotExposeDatabaseURLOrFilePath(t *testing.T) {
	secretURL := "postgres://private-user:private-password@127.0.0.1:5432/not_aicrm_perf?sslmode=disable"
	secretFile := writeDatabaseURLFile(t, secretURL, 0o600)
	for _, args := range [][]string{
		{"--database-url=" + secretURL, "--reset-token=" + resetToken, "--seed=" + seedText},
		{"--database-url-file=" + secretFile, "--reset-token=" + resetToken, "--seed=" + seedText},
	} {
		var stdout, stderr bytes.Buffer
		calls := 0
		exitCode := run(args, &stdout, &stderr, func(context.Context, string, int64) (seedSummary, error) {
			calls++
			return seedSummary{}, nil
		})
		if exitCode != 2 || calls != 0 || stdout.Len() != 0 {
			t.Fatalf("run(%q) exit/calls/stdout = %d/%d/%q", args, exitCode, calls, stdout.String())
		}
		for _, forbidden := range []string{"private-user", "private-password", secretFile, "not_aicrm_perf"} {
			if strings.Contains(stderr.String(), forbidden) {
				t.Fatalf("stderr exposed %q: %q", forbidden, stderr.String())
			}
		}
	}
}

func TestRunPrintsOnlySecretFreeJSONSummary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	seenURL := ""
	path := writeDatabaseURLFile(t, testPerformanceURL, 0o600)
	exitCode := run([]string{
		"--database-url-file=" + path,
		"--reset-token=" + resetToken,
		"--seed=" + seedText,
	}, &stdout, &stderr, func(_ context.Context, databaseURL string, seed int64) (seedSummary, error) {
		seenURL = databaseURL
		return seedSummary{Database: performanceDatabase, Seed: seed, Customers: customerCount, CustomerTags: customerCount * tagsPerCustomer}, nil
	})
	if exitCode != 0 || stderr.Len() != 0 || seenURL != testPerformanceURL {
		t.Fatalf("run() exit/stderr/seenURL = %d/%q/%q", exitCode, stderr.String(), seenURL)
	}
	if strings.Contains(stdout.String(), "private-password") || strings.Contains(stdout.String(), "synthetic@") {
		t.Fatalf("stdout exposed database URL: %q", stdout.String())
	}
	var summary seedSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("stdout is not JSON summary: %v; %q", err, stdout.String())
	}
	if summary.Database != performanceDatabase || summary.Customers != customerCount || summary.CustomerTags != customerCount*tagsPerCustomer {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestDeterministicCustomerRecordsAndHotCohorts(t *testing.T) {
	first := deterministicCustomer(datasetSeed, 1337)
	if first != deterministicCustomer(datasetSeed, 1337) {
		t.Fatal("same seed and index did not produce the same customer")
	}
	if first == deterministicCustomer(datasetSeed+1, 1337) {
		t.Fatal("different seed unexpectedly produced the same customer")
	}
	if ordinary := deterministicCustomer(datasetSeed, 1297); ordinary.name != "kw017" {
		t.Fatalf("ordinary keyword token = %q, want kw017", ordinary.name)
	}
	for _, index := range []int{0, hotCohortPerState, hotCohortPerState*2 - 1} {
		record := deterministicCustomer(datasetSeed, index)
		if record.name != "kw017" || record.ownerStaffID != 7 || record.stageID != 3 || record.channelID != 5 ||
			record.tagIDs != [tagsPerCustomer]int64{11, 12, 13} || !withinInclusive(record.addedAt, addedWindowStart, addedWindowEnd) ||
			!withinInclusive(record.lastInteractAt, interactWindowStart, interactWindowEnd) || !record.updatedAt.Before(queryWatermark) {
			t.Fatalf("hot record %d = %#v", index, record)
		}
	}
	if deterministicCustomer(datasetSeed, 0).isDeleted || !deterministicCustomer(datasetSeed, hotCohortPerState).isDeleted {
		t.Fatal("hot cohorts did not retain active/deleted split")
	}
}

func TestFixedDatasetDistributionTagsAndNeutralExtra(t *testing.T) {
	deleted := 0
	hotActive, hotDeleted := 0, 0
	stages := map[int64]bool{}
	staff := map[int64]bool{}
	channels := map[int64]bool{}
	tags := map[int64]bool{}
	addedBefore, addedWithin, addedAfter := 0, 0, 0
	interactedBefore, interactedWithin, interactedAfter := 0, 0, 0

	for index := 0; index < customerCount; index++ {
		record := deterministicCustomer(datasetSeed, index)
		if record.isDeleted {
			deleted++
		}
		if cohort, isDeleted, _ := hotCohort(index); cohort {
			if isDeleted {
				hotDeleted++
			} else {
				hotActive++
			}
		}
		stages[record.stageID] = true
		staff[record.ownerStaffID] = true
		channels[record.channelID] = true
		seenTags := map[int64]bool{}
		for _, tagID := range record.tagIDs {
			if seenTags[tagID] {
				t.Fatalf("customer %d has duplicate tag %d", index, tagID)
			}
			seenTags[tagID] = true
			tags[tagID] = true
		}
		addedBefore, addedWithin, addedAfter = incrementWindowCounts(record.addedAt, addedWindowStart, addedWindowEnd, addedBefore, addedWithin, addedAfter)
		interactedBefore, interactedWithin, interactedAfter = incrementWindowCounts(record.lastInteractAt, interactWindowStart, interactWindowEnd, interactedBefore, interactedWithin, interactedAfter)
		if !record.updatedAt.Before(queryWatermark) {
			t.Fatalf("customer %d updated after watermark: %s", index, record.updatedAt)
		}
	}
	if deleted != deletedCount || hotActive != hotCohortPerState || hotDeleted != hotCohortPerState {
		t.Fatalf("deleted/hot distribution = %d/%d/%d", deleted, hotActive, hotDeleted)
	}
	if len(stages) != stageCount || len(staff) != staffCount || len(channels) != channelCount || len(tags) != tagCount {
		t.Fatalf("filter distribution stages/staff/channels/tags = %d/%d/%d/%d", len(stages), len(staff), len(channels), len(tags))
	}
	if addedBefore == 0 || addedWithin == 0 || addedAfter == 0 || interactedBefore == 0 || interactedWithin == 0 || interactedAfter == 0 {
		t.Fatalf("time distribution added=%d/%d/%d interacted=%d/%d/%d", addedBefore, addedWithin, addedAfter, interactedBefore, interactedWithin, interactedAfter)
	}

	for _, index := range []int{0, hotCohortPerState, 2001} {
		record := deterministicCustomer(datasetSeed, index)
		var extra map[string]json.RawMessage
		if err := json.Unmarshal([]byte(record.extra), &extra); err != nil {
			t.Fatalf("customer %d extra is invalid JSON: %v", index, err)
		}
		if len(extra) != 2 || extra["synthetic"] == nil || extra["bucket"] == nil {
			t.Fatalf("customer %d extra is not neutral synthetic metadata: %q", index, record.extra)
		}
		lower := strings.ToLower(record.extra)
		for _, forbidden := range []string{"phone", "unionid", "openid", "external_user", "channel_id", "source"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("customer %d extra leaks non-neutral field %q: %q", index, forbidden, record.extra)
			}
		}
	}
}

func writeDatabaseURLFile(t *testing.T, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func errorsIsInvalidArguments(err error) bool {
	return err != nil && err.Error() == errInvalidArguments.Error()
}

func withinInclusive(value, start, end time.Time) bool {
	return !value.Before(start) && !value.After(end)
}

func incrementWindowCounts(value, start, end time.Time, before, within, after int) (int, int, int) {
	switch {
	case value.Before(start):
		return before + 1, within, after
	case value.After(end):
		return before, within, after + 1
	default:
		return before, within + 1, after
	}
}
