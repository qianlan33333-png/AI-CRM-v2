package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestOutboundMediaEffectDetailIsOpaqueAndPIIMinimal(t *testing.T) {
	detail := outboundMediaEffectDetail(42, 7, "reconciled", true, true)
	if detail.ContentPackageID != 42 || detail.EffectID != "eer_7" || detail.State != "reconciled" {
		t.Fatalf("detail = %#v", detail)
	}
	if !detail.ProviderAccepted || !detail.DeliveryProven {
		t.Fatalf("verified delivery detail = %#v", detail)
	}
	for index := 0; index < reflect.TypeOf(detail).NumField(); index++ {
		field := reflect.TypeOf(detail).Field(index)
		if strings.Contains(strings.ToLower(field.Name), "digest") || strings.Contains(strings.ToLower(field.Name), "target") {
			t.Fatalf("detail leaks lookup material through %s", field.Name)
		}
	}
}
