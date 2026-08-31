package main

import (
	"strings"
	"testing"
)

func TestReadSourceProducesStableDigestWithoutReturningRawRows(t *testing.T) {
	first := "unionid,mobile_normalized,mobile_verified,mobile_source,updated_at\nunion-b,13900000000,t,sidebar_bind,2026-08-30T00:00:00Z\nunion-a,13800000000,true,mobile_bind,2026-08-29T00:00:00Z\n"
	second := "unionid,mobile_normalized,mobile_verified,mobile_source,updated_at\nunion-a,13800000000,true,mobile_bind,2026-08-29T00:00:00Z\nunion-b,13900000000,t,sidebar_bind,2026-08-30T00:00:00Z\n"
	rows, digest, err := readSource(strings.NewReader(first))
	if err != nil || len(rows) != 2 || rows[0].unionID != "union-a" || !isSHA256(digest) {
		t.Fatalf("rows=%d digest=%q err=%v", len(rows), digest, err)
	}
	_, secondDigest, err := readSource(strings.NewReader(second))
	if err != nil || digest != secondDigest {
		t.Fatalf("stable digest mismatch: %q / %q / %v", digest, secondDigest, err)
	}
}

func TestReadSourceRejectsDuplicateUnionIDAndInvalidShape(t *testing.T) {
	duplicate := "unionid,mobile_normalized,mobile_verified,mobile_source,updated_at\nu,13800000000,t,a,x\nu,13900000000,t,b,y\n"
	if _, _, err := readSource(strings.NewReader(duplicate)); err == nil {
		t.Fatal("duplicate unionid accepted")
	}
	badHeader := "mobile,unionid,verified,source,updated_at\n13800000000,u,t,a,x\n"
	if _, _, err := readSource(strings.NewReader(badHeader)); err == nil {
		t.Fatal("bad header accepted")
	}
}

func TestReadSourceAcceptsSortedExactOrderEvidence(t *testing.T) {
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	input := "unionid,mobile_normalized,mobile_verified,mobile_source,updated_at,order_reference_digests\n" +
		"union-a,13800000000,true,mobile_bind,2026-08-29T00:00:00Z,transaction:" + digestB + ";merchant:" + digestA + ";merchant:" + digestA + "\n"
	rows, _, err := readSource(strings.NewReader(input))
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	want := []string{"merchant:" + digestA, "transaction:" + digestB}
	if strings.Join(rows[0].orderReferences, "|") != strings.Join(want, "|") {
		t.Fatalf("order references=%v want %v", rows[0].orderReferences, want)
	}
}

func TestReadSourceRejectsInvalidOrderEvidence(t *testing.T) {
	input := "unionid,mobile_normalized,mobile_verified,mobile_source,updated_at,order_reference_digests\n" +
		"union-a,13800000000,true,mobile_bind,2026-08-29T00:00:00Z,merchant:not-a-digest\n"
	if _, _, err := readSource(strings.NewReader(input)); err == nil {
		t.Fatal("invalid order reference accepted")
	}
}

func TestNormalizePhoneAcceptsOnlyCanonicalE164OrMainlandMobile(t *testing.T) {
	for input, want := range map[string]string{
		"13800138000":    "+8613800138000",
		"+8613900139000": "+8613900139000",
		"12800128000":    "",
		"138 0013 8000":  "",
		"":               "",
	} {
		if got := normalizePhone(input); got != want {
			t.Fatalf("normalizePhone(%q)=%q want %q", input, got, want)
		}
	}
}

func TestRunIDAndDigestValidation(t *testing.T) {
	if !validRunID("wli_phone_20260831_current") || validRunID("wli_20260831") {
		t.Fatal("run id validation mismatch")
	}
	if !isSHA256(strings.Repeat("a", 64)) || isSHA256(strings.Repeat("A", 64)) {
		t.Fatal("digest validation mismatch")
	}
}
