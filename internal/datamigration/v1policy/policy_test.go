package v1policy

import "testing"

func TestClassifyExplicitDisposition(t *testing.T) {
	tests := []struct {
		name       string
		want       Disposition
		importable bool
	}{
		{name: "crm_user_identity", want: DispositionCanonical, importable: true},
		{name: "questionnaires", want: DispositionCanonical, importable: true},
		{name: "customer_list_index_next", want: DispositionRebuild},
		{name: "deployment_profile_state", want: DispositionReset},
		{name: "admin_users", want: DispositionManual},
		{name: "  CRM_USER_IDENTITY  ", want: DispositionCanonical, importable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Classify(test.name)
			if got.Disposition != test.want {
				t.Fatalf("Classify(%q).Disposition = %q, want %q", test.name, got.Disposition, test.want)
			}
			if got.Importable != test.importable {
				t.Fatalf("Classify(%q).Importable = %t, want %t", test.name, got.Importable, test.importable)
			}
			if !got.Disposition.valid() {
				t.Fatalf("Classify(%q) returned invalid disposition %q", test.name, got.Disposition)
			}
			if got.LegacyActivationAllowed() {
				t.Fatalf("Classify(%q) allowed legacy activation", test.name)
			}
		})
	}
}

func TestSensitiveAndUnknownTablesFailClosedToArchive(t *testing.T) {
	names := []string{
		"queue_jobs",
		"webhook_events",
		"outbound_tasks",
		"provider_credentials",
		"payment_orders",
		"wechat_pay_refunds",
		"auth_sessions",
		"app_secrets",
		"oauth_tokens",
		"future_table_added_after_freeze",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			got := Classify(name)
			if got.Disposition != DispositionArchived {
				t.Fatalf("Classify(%q).Disposition = %q, want archived", name, got.Disposition)
			}
			if got.Importable || got.LegacyActivationAllowed() {
				t.Fatalf("Classify(%q) produced an active policy: %#v", name, got)
			}
		})
	}
}

func TestFreezeInventoryRequiresExactly272UniqueTables(t *testing.T) {
	if _, err := FreezeInventory(make([]string, ExpectedV1TableCount-1)); err == nil {
		t.Fatal("partial inventory was accepted")
	}

	names := make([]string, ExpectedV1TableCount)
	for index := range names {
		names[index] = "v1_table_" + string(rune('a'+index/26)) + string(rune('a'+index%26))
	}
	inventory, err := FreezeInventory(names)
	if err != nil {
		t.Fatalf("FreezeInventory() error = %v", err)
	}
	if got := len(inventory.Rules()); got != ExpectedV1TableCount {
		t.Fatalf("Rules() count = %d, want %d", got, ExpectedV1TableCount)
	}
	if got := inventory.Rules()[0].Table; got != "v1_table_aa" {
		t.Fatalf("Rules() was not sorted, first table = %q", got)
	}

	names[1] = names[0]
	if _, err := FreezeInventory(names); err == nil {
		t.Fatal("duplicate inventory table was accepted")
	}
}

func TestFreezeInventoryNormalizesNamesBeforeDeduplication(t *testing.T) {
	names := make([]string, ExpectedV1TableCount)
	for index := range names {
		names[index] = "v1_table_" + string(rune('a'+index/26)) + string(rune('a'+index%26))
	}
	names[0] = "  V1_TABLE_AA  "
	names[1] = "v1_table_ab"
	inventory, err := FreezeInventory(names)
	if err != nil {
		t.Fatalf("FreezeInventory() error = %v", err)
	}
	if got := inventory.Rules()[0].Table; got != "v1_table_aa" {
		t.Fatalf("normalized table = %q, want v1_table_aa", got)
	}
}
