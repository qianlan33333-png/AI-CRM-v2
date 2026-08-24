package main

import "testing"

func TestRunRejectsUnsafeRuntimeConfiguration(t *testing.T) {
	getenv := func(string) string { return "same" }
	if err := run([]string{"--mode=full", "--source-manifest=x", "--manifest-sha256=x"}, getenv); err == nil {
		t.Fatal("same source/target accepted")
	}
}
