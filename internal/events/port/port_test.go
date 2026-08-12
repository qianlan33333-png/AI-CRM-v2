package port

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestCustomerMergedEventTypeIsFrozen(t *testing.T) {
	if EvCustomerMerged != "customer.merged" {
		t.Fatalf("EvCustomerMerged = %q; want customer.merged", EvCustomerMerged)
	}
	if (Event{Type: EvCustomerMerged}).Type != "customer.merged" {
		t.Fatal("customer merged event cannot be appended through the event port")
	}
}

func TestCustomerMergedPayloadIsChannelNeutralAndFrozen(t *testing.T) {
	if CustomerMergeAuto != "auto" || CustomerMergeManual != "manual" {
		t.Fatal("customer merge mode values drifted")
	}
	payload := CustomerMergedPayload{PrimaryCustomerID: 7, MergedCustomerID: 9, MergeAuditID: 11, Mode: CustomerMergeAuto, PolicyVersion: "verified_unionid_unique_wecom_v1"}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	want := []string{"merge_audit_id", "merged_customer_id", "mode", "policy_version", "primary_customer_id"}
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("customer.merged payload fields = %v; want %v", got, want)
	}
}
