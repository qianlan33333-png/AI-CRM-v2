package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesConfiguration(t *testing.T) {
	t.Parallel()
	output := filepath.Join(t.TempDir(), "generated")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := run([]string{"--output-dir=" + output, "--tier=s"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if stdout.String() != "aicrm-config: generated tier s configuration\n" || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	for _, name := range []string{"aicrm.env", "postgresql.conf"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunRejectsInvalidArgumentsWithoutEchoingThem(t *testing.T) {
	t.Parallel()
	tests := [][]string{
		nil,
		{"--tier=s"},
		{"--tier=S", "--output-dir=/tmp/output"},
		{"--tier=s", "--tier=m"},
		{"--tier=s", "--output-dir=/tmp/output", "--extra=true"},
		{"--tier", "s"},
	}
	for _, args := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exitCode := run(args, &stdout, &stderr); exitCode != 2 {
			t.Fatalf("run(%q) exit = %d", args, exitCode)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), usageLine) {
			t.Fatalf("run(%q) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
		for _, forbidden := range []string{"/tmp/output", "--extra=true", "medium"} {
			if strings.Contains(stderr.String(), forbidden) {
				t.Fatalf("stderr exposed input %q: %q", forbidden, stderr.String())
			}
		}
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := run([]string{"--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit = %d", exitCode)
	}
	if stdout.String() != usageLine+"\n" || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}
