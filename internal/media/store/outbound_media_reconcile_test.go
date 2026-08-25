package store

import (
	"reflect"
	"strings"
	"testing"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

func TestOutboundMediaReconcileStoreUsesStrictOpaqueIDsAndCanonicalDigests(t *testing.T) {
	if id, err := parseOutboundMediaEffectID("eer_7"); err != nil || id != 7 {
		t.Fatalf("effect id=%d err=%v", id, err)
	}
	for _, value := range []string{"7", "eer_0", "eer_07", "eer_-7", "eer_7x"} {
		if _, err := parseOutboundMediaEffectID(value); err == nil {
			t.Fatalf("accepted noncanonical effect id %q", value)
		}
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	if !validOutboundMediaDigest(digest) || validOutboundMediaDigest(strings.ToUpper(digest)) || validOutboundMediaDigest("sha256:"+strings.Repeat("g", 64)) {
		t.Fatal("digest validation is not canonical")
	}
	for index := 0; index < reflect.TypeOf(mediaapp.OutboundMediaReconciliationReceipt{}).NumField(); index++ {
		field := reflect.TypeOf(mediaapp.OutboundMediaReconciliationReceipt{}).Field(index)
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "target") || strings.Contains(name, "payload") || strings.Contains(name, "recipient") || strings.Contains(name, "body") {
			t.Fatalf("reconciliation receipt leaks PII field %s", field.Name)
		}
	}
}
