// Package v1policy defines the closed, fail-closed disposition policy for
// tables found in the V1 snapshot.
//
// A table is never made active merely because it exists in V1. Only the
// explicit canonical allowlist may be imported as V2 facts. Unknown tables,
// including tables added after the frozen inventory, are archived until a
// later review adds an explicit rule.
package v1policy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ExpectedV1TableCount is the frozen physical-table count from the V1
// inventory. Views and framework metadata are not included.
const ExpectedV1TableCount = 272

// Disposition is the only allowed V1-to-V2 table disposition.
type Disposition string

const (
	DispositionCanonical Disposition = "canonical"
	DispositionArchived  Disposition = "archive"
	DispositionRebuild   Disposition = "rebuild"
	DispositionReset     Disposition = "reset"
	DispositionManual    Disposition = "manual"
)

func (value Disposition) valid() bool {
	switch value {
	case DispositionCanonical, DispositionArchived, DispositionRebuild, DispositionReset, DispositionManual:
		return true
	default:
		return false
	}
}

// Policy is the frozen disposition for one V1 table. Importable is true only
// for canonical facts/definitions. No disposition permits legacy runtime
// activation; reset means a clean V2 runtime may be initialized separately.
type Policy struct {
	Table       string
	Disposition Disposition
	Reason      string
	Importable  bool
}

// Inventory is an immutable-in-practice snapshot of the classified V1 table
// names. Rules returns a copy so callers cannot mutate the frozen result.
type Inventory struct {
	rules []Policy
}

// Rules returns the deterministic, name-sorted classification of the frozen
// inventory.
func (inventory Inventory) Rules() []Policy {
	rules := append([]Policy(nil), inventory.rules...)
	return rules
}

// FreezeInventory accepts exactly the physical V1 inventory. It rejects
// missing/duplicate names and refuses to silently accept a partial scan.
// Unknown names remain archived by Classify; they do not become importable.
func FreezeInventory(names []string) (Inventory, error) {
	if len(names) != ExpectedV1TableCount {
		return Inventory{}, fmt.Errorf("v1 inventory has %d tables, want %d", len(names), ExpectedV1TableCount)
	}

	seen := make(map[string]struct{}, len(names))
	rules := make([]Policy, 0, len(names))
	for _, rawName := range names {
		name := normalize(rawName)
		if name == "" {
			return Inventory{}, errors.New("v1 inventory contains an empty table name")
		}
		if _, exists := seen[name]; exists {
			return Inventory{}, fmt.Errorf("v1 inventory contains duplicate table %q", name)
		}
		seen[name] = struct{}{}
		rules = append(rules, Classify(name))
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Table < rules[j].Table })
	return Inventory{rules: rules}, nil
}

// Classify returns a closed policy for a V1 table name. The order is
// intentional: explicit non-runtime rules are considered first, then
// sensitive-name fail-closed matching, and finally the archive default.
func Classify(tableName string) Policy {
	name := normalize(tableName)
	if rule, ok := explicitPolicies[name]; ok {
		return rule
	}
	if containsSensitiveToken(name) {
		return Policy{
			Table:       name,
			Disposition: DispositionArchived,
			Reason:      "sensitive execution, provider, credential, or runtime table defaults to non-executable archive",
		}
	}
	return Policy{
		Table:       name,
		Disposition: DispositionArchived,
		Reason:      "no explicit V2 mapping is frozen; retain as non-executable archive",
	}
}

// LegacyActivationAllowed is deliberately stricter than disposition. A
// caller may import canonical facts, but no V1 row can resume a job, retry a
// provider call, send a message, charge/refund a payment, or restore a
// session/secret.
func (policy Policy) LegacyActivationAllowed() bool { return false }

var explicitPolicies = func() map[string]Policy {
	policies := make(map[string]Policy)
	add := func(disposition Disposition, reason string, names ...string) {
		for _, name := range names {
			policies[name] = Policy{
				Table:       name,
				Disposition: disposition,
				Reason:      reason,
				Importable:  disposition == DispositionCanonical,
			}
		}
	}

	// These are the frozen MIGRATE candidates that represent V2 facts or
	// definitions. Execution queues, outbound work, and historical audit/event
	// rows are intentionally absent and therefore archive by default.
	add(DispositionCanonical, "frozen V2 fact or definition candidate",
		"ai_audience_package",
		"ai_audience_package_version",
		"audience_rule",
		"audience_rule_version",
		"automation_channel",
		"automation_channel_contact",
		"contact_tags",
		"crm_user_identity",
		"miniprogram_library",
		"owner_role_map",
		"questionnaire_options",
		"questionnaire_questions",
		"questionnaire_submission_answers",
		"questionnaire_submissions",
		"questionnaires",
		"radar_click_events",
		"segments",
		"sidebar_customer_profile_fields",
		"wecom_corp_tag_groups",
		"wecom_corp_tags",
		"wecom_external_contact_identity_map",
	)

	add(DispositionRebuild, "derived/read-model state must be rebuilt from imported V2 facts",
		"admin_wecom_directory_members",
		"ai_audience_hxc_member_usage_projection",
		"ai_audience_member_current",
		"customer_detail_snapshot_next",
		"customer_detail_snapshot_next_shadow",
		"customer_list_index_next",
		"customer_list_index_next_shadow",
		"customer_read_model_refresh_state",
		"customer_timeline_event_next",
		"data_health_snapshot",
		"external_contact_bindings",
		"operation_cycle_metrics",
		"operation_cycle_plan_links",
		"operation_cycle_references",
		"segment_member_snapshots",
		"user_ops_hxc_dashboard_meta",
		"user_ops_hxc_dashboard_snapshot",
		"user_ops_pool_current_next",
		"wecom_group_chat_snapshots",
	)

	add(DispositionReset, "runtime state starts empty and disabled; never resume V1 work",
		"deployment_profile_state",
		"external_effect_job",
		"sync_runs",
	)

	add(DispositionManual, "configuration or account state requires clean V2 re-entry",
		"admin_user_roles",
		"admin_users",
		"app_settings",
		"auth_api_clients",
		"config_releases",
		"mcp_tool_settings",
	)

	return policies
}()

var sensitiveTokens = map[string]struct{}{
	"attempt": {}, "callback": {}, "cookie": {}, "credential": {}, "dispatch": {},
	"hash": {}, "nonce": {}, "oauth": {}, "outbound": {}, "outbox": {},
	"pay": {}, "payment": {}, "password": {}, "provider": {}, "queue": {},
	"receipt": {}, "refund": {}, "replay": {}, "secret": {}, "session": {},
	"token": {}, "webhook": {},
}

func normalize(tableName string) string {
	return strings.ToLower(strings.TrimSpace(tableName))
}

func containsSensitiveToken(tableName string) bool {
	if tableName == "" {
		return false
	}
	for _, token := range strings.Split(tableName, "_") {
		if _, ok := sensitiveTokens[token]; ok {
			return true
		}
	}
	return false
}
