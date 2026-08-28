package v1wecomcontacthistory

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptHistoryPreservesTwoInertSourceFacts(t *testing.T) {
	event, follow := fixtures()
	history := AdaptHistory(
		[]v1archive.ArchivedRow{archiveRow(t, ExternalContactEventLogsTableID, 1, event)},
		[]v1archive.ArchivedRow{archiveRow(t, ExternalContactFollowUsersTableID, 1, follow)},
	)
	if history.SourceCount() != 2 || history.TerminalCount() != 2 {
		t.Fatalf("source conservation changed: %+v", history)
	}
	eventFact := mustCandidate(t, history.EventLogs[0]).Fact
	if eventFact.SourceID != -7 || eventFact.EventTime != nil || eventFact.EventType != "change_external_contact" || eventFact.ChangeType != "delete" || eventFact.RetryCount != -2 || eventFact.ProcessStatus != "failed" || eventFact.IdentitySyncStatus != "skipped" {
		t.Fatalf("event source scalars changed: %+v", eventFact)
	}
	if eventFact.CreatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:30:00.123456+08:00" || eventFact.UpdatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:29:00.123456+08:00" || eventFact.PayloadJSONDigest == (OpaqueDigest{}) || eventFact.IdentitySyncResponseDigest == (OpaqueDigest{}) || eventFact.Source.SourceKeyDigest == (OpaqueDigest{}) {
		t.Fatalf("event timestamps, JSON null, or archive envelope changed: %+v", eventFact)
	}
	followFact := mustCandidate(t, history.FollowUsers[0]).Fact
	if followFact.SourceID != 0 || followFact.AddWay != nil || followFact.CreateTime == nil || *followFact.CreateTime != -8 || !followFact.IsPrimary || followFact.RelationStatus != "active" || followFact.State != "legacy-state" {
		t.Fatalf("follow source scalars changed: %+v", followFact)
	}
	if followFact.FirstSeenAt.Format(time.RFC3339Nano) != "2026-08-28T09:31:00.123456+08:00" || followFact.LastSeenAt.Format(time.RFC3339Nano) != "2026-08-28T09:30:00.123456+08:00" || followFact.RawFollowUserDigest == (OpaqueDigest{}) {
		t.Fatalf("follow timestamps or JSON null changed: %+v", followFact)
	}
}

func TestAdaptHistoryQuarantinesInvalidArchiveShapeAndRedaction(t *testing.T) {
	event, follow := fixtures()
	for _, test := range []struct {
		name  string
		table string
		value map[string]any
		field string
		bad   any
	}{
		{"event id type", ExternalContactEventLogsTableID, event, "id", "-7"},
		{"event nullable number type", ExternalContactEventLogsTableID, event, "event_time", "unknown"},
		{"event required time", ExternalContactEventLogsTableID, event, "created_at", nil},
		{"follow nullable number type", ExternalContactFollowUsersTableID, follow, "add_way", "unknown"},
		{"follow required bool", ExternalContactFollowUsersTableID, follow, "is_primary", "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := clone(test.value)
			copy[test.field] = test.bad
			result := adaptForTable(test.table, archiveRow(t, test.table, 1, copy))
			if resultDisposition(result) != DispositionQuarantine {
				t.Fatalf("invalid source shape accepted: %+v", result)
			}
		})
	}

	row := archiveRow(t, ExternalContactEventLogsTableID, 1, event)
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = ExternalContactFollowUsersTableID },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{`) },
		func(value *v1archive.ArchivedRow) { value.RedactedFields = []string{"payload_json.secret"} },
	} {
		changed := row
		mutate(&changed)
		if resultDisposition(AdaptHistory([]v1archive.ArchivedRow{changed}, nil).EventLogs[0]) != DispositionQuarantine {
			t.Fatal("invalid archive envelope or redaction was accepted")
		}
	}
}

func TestAdaptHistoryAcceptsJSONNullButNotMissingSourceField(t *testing.T) {
	event, follow := fixtures()
	event["payload_json"] = nil
	event["identity_sync_response_json"] = nil
	follow["raw_follow_user"] = nil
	history := AdaptHistory(
		[]v1archive.ArchivedRow{archiveRow(t, ExternalContactEventLogsTableID, 1, event)},
		[]v1archive.ArchivedRow{archiveRow(t, ExternalContactFollowUsersTableID, 1, follow)},
	)
	if history.EventLogs[0].Disposition != DispositionCandidate || history.FollowUsers[0].Disposition != DispositionCandidate {
		t.Fatalf("JSON literal null was confused with a missing source value: %+v", history)
	}
	delete(event, "payload_json")
	if resultDisposition(AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, ExternalContactEventLogsTableID, 1, event)}, nil).EventLogs[0]) != DispositionQuarantine {
		t.Fatal("missing JSONB field was accepted")
	}
}

func TestAdaptHistoryQuarantinesDuplicateSourceIDsPerTableWithoutCollapsingRelations(t *testing.T) {
	event, follow := fixtures()
	eventRows := []v1archive.ArchivedRow{
		archiveRow(t, ExternalContactEventLogsTableID, 1, event),
		archiveRow(t, ExternalContactEventLogsTableID, 2, event),
	}
	otherEvent := clone(event)
	otherEvent["id"] = int64(0)
	history := AdaptHistory(eventRows, []v1archive.ArchivedRow{
		archiveRow(t, ExternalContactFollowUsersTableID, 1, follow),
		archiveRow(t, ExternalContactFollowUsersTableID, 2, clone(follow)),
	})
	for _, row := range history.EventLogs {
		if row.Disposition != DispositionQuarantine || row.Reason != "wecom_external_contact_event_logs_source_ambiguous" || row.Fact != nil {
			t.Fatalf("duplicate event source ID not quarantined: %+v", row)
		}
	}
	for _, row := range history.FollowUsers {
		if row.Disposition != DispositionQuarantine || row.Reason != "wecom_external_contact_follow_users_source_ambiguous" || row.Fact != nil {
			t.Fatalf("duplicate follow source ID not quarantined: %+v", row)
		}
	}
	// Same signed source ID in distinct source tables is not a parent relation.
	history = AdaptHistory([]v1archive.ArchivedRow{archiveRow(t, ExternalContactEventLogsTableID, 1, otherEvent)}, []v1archive.ArchivedRow{archiveRow(t, ExternalContactFollowUsersTableID, 1, follow)})
	if history.EventLogs[0].Disposition != DispositionCandidate || history.FollowUsers[0].Disposition != DispositionCandidate {
		t.Fatalf("cross-table source IDs were incorrectly merged: %+v", history)
	}
}

func TestAdaptHistoryDoesNotSerializePrivateSourceMaterial(t *testing.T) {
	event, follow := fixtures()
	history := AdaptHistory(
		[]v1archive.ArchivedRow{archiveRow(t, ExternalContactEventLogsTableID, 1, event)},
		[]v1archive.ArchivedRow{archiveRow(t, ExternalContactFollowUsersTableID, 1, follow)},
	)
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"corp-private", "external-private", "user-private", "oper-private", "event-key-private", "xml-private", "payload-private", "error-private", "remark-private", "description-private"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private source material leaked from candidate JSON: %q", private)
		}
	}
	if history.EventLogs[0].Fact.PayloadXMLDigest == (OpaqueDigest{}) || history.EventLogs[0].Fact.ErrorMessageDigest == (OpaqueDigest{}) || history.FollowUsers[0].Fact.RemarkDigest == (OpaqueDigest{}) {
		t.Fatal("private material did not leave opaque digests")
	}
}

func fixtures() (map[string]any, map[string]any) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	event := map[string]any{
		"id": int64(-7), "corp_id": "corp-private", "event_type": "change_external_contact", "change_type": "delete", "external_userid": "external-private", "user_id": "user-private", "event_time": nil, "event_key": "event-key-private", "payload_xml": "xml-private", "payload_json": nil, "process_status": "failed", "retry_count": int32(-2), "error_message": "error-private", "created_at": stamp, "updated_at": stamp.Add(-time.Minute), "identity_sync_status": "skipped", "identity_sync_error_code": "error-code-private", "identity_sync_error_message": "error-private", "identity_sync_response_json": nil,
	}
	follow := map[string]any{
		"id": int64(0), "corp_id": "corp-private", "external_userid": "external-private", "user_id": "user-private", "relation_status": "active", "is_primary": true, "remark": "remark-private", "description": "description-private", "add_way": nil, "state": "legacy-state", "oper_userid": "oper-private", "createtime": int64(-8), "raw_follow_user": nil, "first_seen_at": stamp.Add(time.Minute), "last_seen_at": stamp, "created_at": stamp, "updated_at": stamp.Add(-time.Minute),
	}
	return event, follow
}

func archiveRow(t *testing.T, table string, ordinal int64, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(table + "/key/" + string(rune(ordinal)))),
		PayloadHMAC:   sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(table + "/fields/" + string(rune(ordinal)))), Payload: payload,
	}
}

func clone(value map[string]any) map[string]any {
	copy := make(map[string]any, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}

func mustCandidate[T any](t *testing.T, value Result[T]) Result[T] {
	t.Helper()
	if value.Disposition != DispositionCandidate || value.Fact == nil {
		t.Fatalf("expected candidate: %+v", value)
	}
	return value
}

func adaptForTable(table string, row v1archive.ArchivedRow) any {
	if table == ExternalContactEventLogsTableID {
		return adaptEventLog(row, 1)
	}
	return adaptFollowUser(row, 1)
}

func resultDisposition(value any) Disposition {
	switch value := value.(type) {
	case Result[ExternalContactEventLogFact]:
		return value.Disposition
	case Result[ExternalContactFollowUserFact]:
		return value.Disposition
	default:
		return ""
	}
}
