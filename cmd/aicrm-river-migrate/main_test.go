package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

const testDatabaseURL = "postgres://user:private-value@127.0.0.1:5432/aicrm?sslmode=disable"

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("AICRM_DATABASE_URL", testDatabaseURL)
	t.Setenv("AICRM_HTTP_LISTEN_ADDRESS", ":8080")
	t.Setenv("AICRM_API_PGX_MAX_CONNS", "10")
	t.Setenv("AICRM_IDENTITY_HMAC_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
}

func TestRunMigratesUpWithoutExposingDatabaseURL(t *testing.T) {
	setValidEnvironment(t)
	var stdout, stderr bytes.Buffer
	seen := ""
	exitCode := run([]string{"--direction=up"}, &stdout, &stderr, func(ctx context.Context, databaseURL string) error {
		if deadline, ok := ctx.Deadline(); !ok || deadline.IsZero() {
			t.Fatal("migration context has no deadline")
		}
		seen = databaseURL
		return nil
	})
	if exitCode != 0 || seen != testDatabaseURL || stderr.Len() != 0 ||
		stdout.String() != "aicrm-river-migrate: River migration up completed\n" {
		t.Fatalf("run() exit/stdout/stderr/seen = %d/%q/%q/%q", exitCode, stdout.String(), stderr.String(), seen)
	}
	if strings.Contains(stdout.String(), "private-value") {
		t.Fatal("stdout exposed database credentials")
	}
}

func TestRunFailsClosedWithStableErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		set  bool
		err  error
		want int
	}{
		{name: "missing argument", want: 2},
		{name: "down forbidden", args: []string{"--direction=down"}, set: true, want: 2},
		{name: "invalid config", args: []string{"--direction=up"}, want: 1},
		{name: "migration error", args: []string{"--direction=up"}, set: true, err: errors.New("private-value"), want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.set {
				setValidEnvironment(t)
			} else {
				t.Setenv("AICRM_DATABASE_URL", "")
				t.Setenv("AICRM_HTTP_LISTEN_ADDRESS", "")
				t.Setenv("AICRM_API_PGX_MAX_CONNS", "")
				t.Setenv("AICRM_IDENTITY_HMAC_KEY", "")
			}
			var stdout, stderr bytes.Buffer
			calls := 0
			exitCode := run(test.args, &stdout, &stderr, func(context.Context, string) error {
				calls++
				return test.err
			})
			if exitCode != test.want || stdout.Len() != 0 || strings.Contains(stderr.String(), "private-value") ||
				(test.name != "migration error" && calls != 0) {
				t.Fatalf("run() exit/stdout/stderr/calls = %d/%q/%q/%d", exitCode, stdout.String(), stderr.String(), calls)
			}
		})
	}
}

func TestRunRejectsNilDependencies(t *testing.T) {
	setValidEnvironment(t)
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"--direction=up"}, nil, &stderr, migrateUp); exitCode != 2 {
		t.Fatalf("run() with nil stdout exit = %d", exitCode)
	}
	if exitCode := run([]string{"--direction=up"}, &stdout, nil, migrateUp); exitCode != 2 {
		t.Fatalf("run() with nil stderr exit = %d", exitCode)
	}
	if exitCode := run([]string{"--direction=up"}, &stdout, &stderr, nil); exitCode != 2 {
		t.Fatalf("run() with nil migration runner exit = %d", exitCode)
	}
}
