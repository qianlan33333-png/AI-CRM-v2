package main

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAudienceActivityParentSourceKeyUsesFrozenSingleIDJSON(t *testing.T) {
	key := bytes.Repeat([]byte{0x63}, sha256.Size)
	got, err := audienceActivityParentSourceKey(key, v1audiencehistory.PackagesTableID, 42)
	if err != nil {
		t.Fatal(err)
	}
	want, err := v1archive.SourceKeyHMAC(key, "ai_audience_package", []byte("[42]"))
	if err != nil || got != want {
		t.Fatalf("parent key=%x want=%x err=%v", got, want, err)
	}
	wrong, err := v1archive.SourceKeyHMAC(key, "ai_audience_package", []byte("[\"42\"]"))
	if err != nil || got == wrong {
		t.Fatalf("string source id was accepted: key=%x wrong=%x err=%v", got, wrong, err)
	}
	if _, err = audienceActivityParentSourceKey(key, v1audiencehistory.PackagesTableID, 0); err == nil {
		t.Fatal("zero parent source id accepted")
	}
}
