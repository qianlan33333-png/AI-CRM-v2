package v1channel

import (
	"encoding/json"
	"testing"
	"time"
)

var channelCandidateTime = time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)

func TestConvertAutomationChannelProducesOnlyInactiveLocalWhitelist(t *testing.T) {
	decision := ConvertAutomationChannel(validChannelRowForTest(), 17)
	if decision.Disposition != Candidate || decision.Reason != "" || decision.Candidate == nil {
		t.Fatalf("decision = %#v", decision)
	}
	got := *decision.Candidate
	if got.SourceKey != "automation_channel:49" || got.Code != "v1-course" || got.Name != "课程渠道" ||
		got.CreatedAt != channelCandidateTime || got.UpdatedAt != channelCandidateTime.Add(time.Hour) || got.MigrationActorID != 17 ||
		got.Config != (LocalInactiveConfig{SchemaVersion: 1, ChannelType: "qrcode", CarrierType: "qrcode", ChannelCode: "v1-course", ChannelName: "课程渠道", Status: "inactive"}) ||
		got.ArchiveConfigDigest != "sha256:020b37fc2bb77634c50387ca5d9af57e6feccca79204575ff70177707d0dce38" || !got.ArchivePayloadNeeded {
		t.Fatalf("candidate = %#v", got)
	}
	encoded, err := json.Marshal(got.Config)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"scene_value", "qr_url", "link_url", "final_url", "welcome_message", "welcome_image_library_ids", "entry_tag_id", "owner_staff_id", "assignees"} {
		if _, found := fields[forbidden]; found {
			t.Fatalf("config leaked forbidden %q: %s", forbidden, encoded)
		}
	}
}

func TestConvertAutomationChannelFailsClosed(t *testing.T) {
	tests := []struct {
		name, reason string
		row          AutomationChannelRow
		actor        int64
		disposition  Disposition
	}{
		{name: "missing actor", reason: ReasonInvalidChannelDefinition, row: validChannelRowForTest(), disposition: Quarantine},
		{name: "missing archive payload", reason: ReasonMissingEncryptedArchivePayload, row: AutomationChannelRow{SourceID: 49, ChannelCode: "v1-course", ChannelName: "课程渠道", ChannelType: "qrcode", CarrierType: "qrcode", CreatedAt: channelCandidateTime, UpdatedAt: channelCandidateTime.Add(time.Hour)}, actor: 17, disposition: Quarantine},
		{name: "provider kind", reason: ReasonUnsupportedChannelKind, row: AutomationChannelRow{SourceID: 49, ChannelCode: "v1-course", ChannelName: "课程渠道", ChannelType: "qrcode", CarrierType: "provider_custom", CreatedAt: channelCandidateTime, UpdatedAt: channelCandidateTime.Add(time.Hour), ArchivePayload: []byte("encrypted-source-payload")}, actor: 17, disposition: Archive},
		{name: "bad timestamps", reason: ReasonInvalidChannelDefinition, row: AutomationChannelRow{SourceID: 49, ChannelCode: "v1-course", ChannelName: "课程渠道", ChannelType: "qrcode", CarrierType: "qrcode", CreatedAt: channelCandidateTime, UpdatedAt: channelCandidateTime.Add(-time.Hour), ArchivePayload: []byte("encrypted-source-payload")}, actor: 17, disposition: Quarantine},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConvertAutomationChannel(tc.row, tc.actor)
			if got.Disposition != tc.disposition || got.Reason != tc.reason || got.Candidate != nil {
				t.Fatalf("decision = %#v", got)
			}
		})
	}
}

func TestClassifyAuxiliaryTableNeverCreatesCandidate(t *testing.T) {
	tests := map[string]struct {
		disposition Disposition
		reason      string
	}{
		"automation_channel_assignee":         {Blocked, ReasonStaffMappingRequired},
		"automation_channel_contact":          {Blocked, ReasonHistoricalEntryProjectionNeeded},
		"automation_channel_entry_effect_log": {Archive, ReasonProviderEffectHistoryArchive},
		"automation_channel_entry_runtime":    {Archive, ReasonCallbackRuntimeArchive},
		"automation_channel_qrcode_asset":     {Archive, ReasonProviderAssetArchive},
		"automation_channel_scene_alias":      {Archive, ReasonCallbackAliasArchive},
		"channel_welcome_effect_dependency":   {Archive, ReasonWelcomeDependencyArchive},
		"channel_welcome_effect_graph":        {Archive, ReasonWelcomeExecutionArchive},
		"unknown":                             {Quarantine, ReasonUnknownChannelSourceTable},
	}
	for table, want := range tests {
		t.Run(table, func(t *testing.T) {
			got := ClassifyAuxiliaryTable(table)
			if got.Disposition != want.disposition || got.Reason != want.reason || got.Candidate != nil {
				t.Fatalf("decision = %#v", got)
			}
		})
	}
}

func validChannelRowForTest() AutomationChannelRow {
	return AutomationChannelRow{
		SourceID:       49,
		ChannelCode:    "v1-course",
		ChannelName:    "课程渠道",
		ChannelType:    "qrcode",
		CarrierType:    "qrcode",
		CreatedAt:      channelCandidateTime,
		UpdatedAt:      channelCandidateTime.Add(time.Hour),
		ArchivePayload: []byte("encrypted-source-payload"),
	}
}
