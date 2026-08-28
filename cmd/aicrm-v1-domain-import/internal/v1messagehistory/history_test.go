package v1messagehistory

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func messageFixture(t *testing.T, change func(map[string]any)) json.RawMessage {
	t.Helper()
	m := map[string]any{
		"id": int64(9007199254740993), "seq": int64(-2), "msgid": " private-message ",
		"chat_type": "private", "owner_userid": " owner ", "sender": " private-sender ", "receiver": " private-receiver ",
		"msgtype": "text", "content": " 原文\n13800138000\t<script>not-html</script> ",
		"send_time": "2026-08-27T21:36:01.123456789+08:00", "created_at": "2026-08-27T13:36:01.390537Z",
		"raw_payload": `{"roomid":"private-room","tolist":["private-recipient"],"secret":"private-token"}`, "unionid": " private-union ",
	}
	if change != nil {
		change(m)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	return raw
}

func TestMessagePreservesEverySourceFieldAndPrivateBody(t *testing.T) {
	raw := messageFixture(t, nil)
	r := AdaptMessage(raw)
	if r.Disposition != Candidate || r.Reason != "" || r.Fact == nil {
		t.Fatal("valid_source_rejected")
	}
	var expected Source
	if json.Unmarshal(raw, &expected) != nil || !reflect.DeepEqual(expected, r.Fact.Source) {
		t.Fatal("source_field_changed")
	}
	if r.Fact.Source.ID != 9007199254740993 || *r.Fact.Source.Seq != -2 {
		t.Fatal("source_integer_lost")
	}
	want := time.Date(2026, 8, 27, 13, 36, 1, 123456000, time.UTC)
	if r.Fact.SentAt == nil || !r.Fact.SentAt.Equal(want) || r.Fact.SentAt.Location() != time.UTC {
		t.Fatal("explicit_timestamp_not_normalized")
	}
	for _, value := range []any{r, r.Fact} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal("safe_result_encode_failed")
		}
		for _, forbidden := range []string{"private-", "private-union", "13800138000", "原文", "not-html", "raw_payload", "9007199254740993"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatal("source_exposed_in_json")
			}
		}
	}
	raw[0] = '['
	if r.Raw[0] != '{' {
		t.Fatal("raw_source_alias_aliases_input")
	}
}

func TestMessageNullableAndEmptyFactsRemainDistinct(t *testing.T) {
	r := AdaptMessage(messageFixture(t, func(m map[string]any) {
		for _, field := range strings.Fields(nullableFields) {
			m[field] = nil
		}
		m["unionid"], m["sender"], m["msgid"] = "", "", ""
		m["chat_type"], m["msgtype"] = "legacy-scene", "file"
	}))
	if r.Disposition != Candidate || r.Fact == nil {
		t.Fatal("valid_nullable_source_rejected")
	}
	f := r.Fact.Source
	if f.Seq != nil || f.OwnerUserID != nil || f.Receiver != nil || f.Content != nil || f.UnionID != "" || f.ChatType != "legacy-scene" {
		t.Fatal("nullable_source_changed")
	}
	other := AdaptMessage(messageFixture(t, func(m map[string]any) { m["content"] = "" }))
	if other.Fact == nil || other.Fact.Source.Content == nil || *other.Fact.Source.Content != "" {
		t.Fatal("empty_content_became_null")
	}
}

func TestMessageUnknownTimeRemainsPendingWithOriginalBody(t *testing.T) {
	for _, value := range []string{"2026-08-27T13:36:01", "1787837761", "1787837761000", "2026-08-27 13:36:01 CST", "", "2026-99-99T13:36:01Z", "2026-02-29 13:36:01", "2026-04-31 13:36:01", "2026-08-27 24:00:00", "2026-08-27 13:60:01", "2026-08-27 13:36:60", "0000-01-01 00:00:00", "2026-8-27 13:36:01", "2026-08-27 13:36:01.123", " 2026-08-27 13:36:01", "2026-08-27 13:36:01 "} {
		raw := messageFixture(t, func(m map[string]any) { m["send_time"] = value })
		r := AdaptMessage(raw)
		if r.Disposition != Pending || r.Reason != "message_send_time_unmapped" || r.Fact == nil || r.Fact.SendTimeBasis != SendTimeUnmapped || r.Fact.SentAt != nil || r.Fact.Source.SendTime != value || r.Fact.Source.Content == nil || string(r.Raw) != string(raw) {
			t.Fatal("unknown_time_was_inferred_or_source_lost")
		}
	}
	r := AdaptMessage(messageFixture(t, func(m map[string]any) { m["created_at"] = "2026-08-27 13:36:01" }))
	if r.Disposition != Pending || r.Reason != "message_created_time_unmapped" || r.Fact == nil {
		t.Fatal("created_timezone_inferred")
	}
	for _, value := range []string{"2026-08-27 13:36:01.123456+00", "2026-08-27 21:36:01.123456+08:00", "2026-08-27 21:36:01.123456+0800"} {
		r := AdaptMessage(messageFixture(t, func(m map[string]any) { m["send_time"] = value }))
		if r.Disposition != Candidate || r.Fact.SendTimeBasis != SendTimeExplicitOffset || r.Fact.SentAt == nil || r.Fact.Source.SendTime != value {
			t.Fatal("explicit_timezone_rejected_or_text_changed")
		}
	}
}

func TestMessageCivilTimePreservesClockWithoutInventInstant(t *testing.T) {
	for _, value := range []string{"2026-08-27 13:36:01", "2024-02-29 23:59:59", "2000-02-29 00:00:00", "0001-01-01 00:00:00"} {
		raw := messageFixture(t, func(m map[string]any) { m["send_time"] = value })
		r := AdaptMessage(raw)
		if r.Disposition != Candidate || r.Reason != "" || r.Fact == nil || r.Fact.SendTimeBasis != SendTimeCivilUnzoned || r.Fact.SentAt != nil || r.Fact.Source.SendTime != value || string(r.Raw) != string(raw) {
			t.Fatal("civil_clock_changed_or_instant_invent")
		}
		encoded, err := json.Marshal(r.Fact)
		if err != nil || !strings.Contains(string(encoded), `"send_time_basis":"civil_unzoned"`) || !strings.Contains(string(encoded), `"sent_at":null`) || strings.Contains(string(encoded), value) {
			t.Fatal("civil_result_exposes_source_or_fake_instant")
		}
	}
	rows := []json.RawMessage{
		messageFixture(t, func(m map[string]any) { m["id"], m["send_time"] = 1, "2026-08-27 13:36:01" }),
		messageFixture(t, func(m map[string]any) { m["id"] = 2 }),
		messageFixture(t, func(m map[string]any) { m["id"], m["send_time"] = 3, "2026-02-29 13:36:01" }),
	}
	var stream StreamSummary
	for _, raw := range rows {
		stream.Add(AdaptMessage(raw))
	}
	batch := Summarize(AdaptHistory(rows))
	if !reflect.DeepEqual(stream.Summary, batch) || batch.CivilSendTime != 1 || batch.ParsedSendTime != 1 || batch.UnmappedSendTime != 1 || batch.Candidates != 2 || batch.Pending != 1 || batch.Decoded != batch.CivilSendTime+batch.ParsedSendTime+batch.UnmappedSendTime {
		t.Fatal("time_basis_summary_not_conserved")
	}
}

func TestMessageMalformedAndRequiredFieldBoundaries(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, []byte("null"), []byte("[]"), []byte("{"), []byte("{} {}"), {0xff}} {
		r := AdaptMessage(raw)
		if r.Disposition != Invalid || r.Reason != "message_json_invalid" || r.Fact != nil {
			t.Fatal("invalid_json_accepted")
		}
	}
	for _, field := range strings.Fields(requiredFields + " " + nullableFields) {
		r := AdaptMessage(messageFixture(t, func(m map[string]any) { delete(m, field) }))
		if r.Disposition != Invalid {
			t.Fatal("missing_field_accepted")
		}
	}
	for _, field := range strings.Fields(requiredFields) {
		r := AdaptMessage(messageFixture(t, func(m map[string]any) { m[field] = nil }))
		if r.Disposition != Invalid {
			t.Fatal("null_required_field_accepted")
		}
	}
	for _, change := range []func(map[string]any){
		func(m map[string]any) { m["id"] = 0 }, func(m map[string]any) { m["id"] = 1.5 },
		func(m map[string]any) { m["id"] = "1" }, func(m map[string]any) { m["seq"] = 1.5 },
		func(m map[string]any) { m["content"] = map[string]any{} }, func(m map[string]any) { m["receiver"] = []string{} },
	} {
		if AdaptMessage(messageFixture(t, change)).Disposition != Invalid {
			t.Fatal("invalid_field_type_accepted")
		}
	}
}

func TestMessageHistoryKeepsDuplicatesAndSourceConservation(t *testing.T) {
	a := messageFixture(t, func(m map[string]any) { m["id"] = 1 })
	b := messageFixture(t, func(m map[string]any) { m["id"] = 2 })
	rows := AdaptHistory([]json.RawMessage{a, b})
	if len(rows) != 2 || rows[0].Disposition != Candidate || rows[1].Disposition != Candidate {
		t.Fatal("duplicate_msgid_was_dropped")
	}
	rows = AdaptHistory([]json.RawMessage{a, b, a})
	if len(rows) != 3 || rows[0].Reason != "message_source_id_duplicate" || rows[2].Reason != "message_source_id_duplicate" || rows[0].Fact == nil || rows[2].Fact == nil || rows[1].Disposition != Candidate {
		t.Fatal("source_pk_collision_not_preserved_and_rejected")
	}
}

func TestMessageSummaryContainsOnlyFixedCounts(t *testing.T) {
	rows := AdaptHistory([]json.RawMessage{
		messageFixture(t, func(m map[string]any) { m["id"] = 1 }),
		messageFixture(t, func(m map[string]any) {
			m["id"], m["chat_type"], m["msgtype"], m["unionid"], m["send_time"] = 2, "group", "image", "", "1787837761"
			m["seq"], m["receiver"], m["owner_userid"], m["content"], m["raw_payload"] = nil, nil, nil, nil, "not-json"
		}),
		messageFixture(t, func(m map[string]any) {
			m["id"], m["msgid"], m["content"] = 3, "different", strings.Repeat("汉", 10001)
		}),
		[]byte("{"),
	})
	s := Summarize(rows)
	if s.Rows != 4 || s.Decoded != 3 || s.Candidates != 2 || s.Pending != 1 || s.Invalid != 1 || s.Private != 2 || s.Group != 1 || s.Text != 2 || s.Image != 1 || s.NullContent != 1 || s.ControlContent != 1 || s.PaddedContent != 1 || s.NullByteContent != 0 || s.NullByteSendTime != 0 || s.NullByteChatOrType != 0 || s.OverNativeBodyLimit != 1 || s.OverSidebarBodyLimit != 1 || s.EmptyUnionID != 1 || s.MissingOwner != 1 || s.MissingReceiver != 1 || s.NullSeq != 1 || s.DuplicateMessageIDGroups != 1 || s.DuplicateMessageIDExtraRows != 1 || s.ParsedSendTime != 2 || s.UnmappedSendTime != 1 || s.NumericSendTime != 1 || s.InvalidPayloadJSON != 1 || s.ObjectPayload != 2 || s.PayloadRoomID != 2 || s.PayloadToList != 2 {
		t.Fatal("shape_summary_wrong")
	}
	encoded, _ := json.Marshal(s)
	if strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), "13800138000") || strings.Contains(string(encoded), "汉") {
		t.Fatal("summary_exposed_source")
	}
}

func TestMessageSummaryCountsNULBytesWithoutChangingCandidateRules(t *testing.T) {
	rows := AdaptHistory([]json.RawMessage{
		messageFixture(t, func(m map[string]any) { m["id"], m["content"] = 1, "body\x00tail" }),
		messageFixture(t, func(m map[string]any) {
			m["id"], m["content"], m["send_time"] = 2, "plain", "2026-08-27T21:36:01+08:00\x00"
		}),
		messageFixture(t, func(m map[string]any) {
			m["id"], m["content"], m["chat_type"], m["msgtype"] = 3, "plain", "private\x00", "text\x00"
		}),
	})
	summary := Summarize(rows)
	if summary.Rows != 3 || summary.Decoded != 3 || summary.NullByteContent != 1 || summary.NullByteSendTime != 1 || summary.NullByteChatOrType != 1 || summary.ControlContent != 1 || rows[0].Disposition != Candidate || rows[1].Disposition != Pending || rows[2].Disposition != Candidate {
		t.Fatalf("unexpected NUL summary=%#v dispositions=%s/%s/%s", summary, rows[0].Disposition, rows[1].Disposition, rows[2].Disposition)
	}
}

func TestMessageStreamSummaryMatchesBatchAtEveryPrefix(t *testing.T) {
	fixture := func(id int64, change func(map[string]any)) json.RawMessage {
		return messageFixture(t, func(m map[string]any) {
			m["id"] = id
			if change != nil {
				change(m)
			}
		})
	}
	payloads := []json.RawMessage{
		fixture(1, nil),
		fixture(2, func(m map[string]any) { m["send_time"] = "unknown" }),
		fixture(0, nil),
		fixture(3, func(m map[string]any) { m["msgid"], m["content"] = "another", strings.Repeat("正文", 16000) }),
		[]byte("{"),
		fixture(4, func(m map[string]any) {
			m["content"], m["receiver"], m["owner_userid"], m["seq"], m["send_time"] = nil, nil, nil, nil, "2026-08-27 13:36:01"
		}),
		fixture(1, func(m map[string]any) { m["send_time"] = "unknown" }),
		fixture(2, nil),
		fixture(0, nil),
		fixture(1, nil),
		fixture(5, func(m map[string]any) { m["created_at"], m["msgtype"] = "unknown", "file" }),
	}
	for pass := 0; pass < 2; pass++ {
		var stream StreamSummary
		for i, raw := range payloads {
			stream.Add(AdaptMessage(raw))
			batch := Summarize(AdaptHistory(payloads[:i+1]))
			if !reflect.DeepEqual(stream.Summary, batch) {
				t.Fatalf("stream_batch_mismatch pass=%d prefix=%d", pass, i+1)
			}
			if stream.Summary.Rows != stream.Summary.Candidates+stream.Summary.Pending+stream.Summary.Invalid {
				t.Fatal("stream_row_conservation_failed")
			}
		}
		if stream.Summary.Reasons["message_source_id_duplicate"] != 7 || stream.Summary.Reasons["message_send_time_unmapped"] != 0 || stream.Summary.Reasons["message_source_id_invalid"] != 0 || stream.Summary.Reasons["message_created_time_unmapped"] != 1 {
			t.Fatal("duplicate_reclassification_wrong")
		}
		for i, j := 0, len(payloads)-1; i < j; i, j = i+1, j-1 {
			payloads[i], payloads[j] = payloads[j], payloads[i]
		}
	}
}

func TestMessageStreamRetainsOnlyIdentifierMetadata(t *testing.T) {
	var stream StreamSummary
	for i := 1; i <= 20; i++ {
		row := AdaptMessage(messageFixture(t, func(m map[string]any) {
			m["id"], m["content"] = i, strings.Repeat("private-body", 50000)
		}))
		stream.Add(row)
		// The accumulator must not retain references to the row or its source.
		row.Raw = nil
		row.Fact.Source = Source{}
	}
	if stream.Summary.Rows != 20 || stream.Summary.Candidates != 20 || stream.Summary.OverNativeBodyLimit != 20 || stream.Summary.DuplicateMessageIDGroups != 1 || stream.Summary.DuplicateMessageIDExtraRows != 19 || len(stream.sourceIDs) != 20 || len(stream.messageIDs) != 1 {
		t.Fatal("stream_metadata_count_wrong")
	}
	encoded, err := json.Marshal(stream)
	if err != nil || strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), `"Raw"`) || strings.Contains(string(encoded), `"Fact"`) {
		t.Fatal("stream_retained_or_exposed_source")
	}
	for _, state := range stream.sourceIDs {
		if state.disposition != Candidate || state.reason != "" || state.duplicate {
			t.Fatal("stream_source_state_wrong")
		}
	}
}
