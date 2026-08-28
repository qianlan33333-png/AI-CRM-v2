// Package v1messagehistory preserves frozen message facts without assigning a
// customer, changing message content, or calling a Provider or target writer.
package v1messagehistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MessageTable = "public/archived_messages"
const requiredFields = "id msgid chat_type sender msgtype send_time raw_payload created_at unionid"
const nullableFields = "seq owner_userid receiver content"

type Disposition string

const (
	Candidate Disposition = "historical_candidate"
	Pending   Disposition = "pending"
	Invalid   Disposition = "invalid"
)

// Source contains source facts, not V2 identifiers. It must never be logged or
// returned by an API. Raw text, including whitespace and NULL, is unchanged.
type Source struct {
	ID          int64   `json:"id"`
	Seq         *int64  `json:"seq"`
	MessageID   string  `json:"msgid"`
	ChatType    string  `json:"chat_type"`
	OwnerUserID *string `json:"owner_userid"`
	Sender      string  `json:"sender"`
	Receiver    *string `json:"receiver"`
	MessageType string  `json:"msgtype"`
	Content     *string `json:"content"`
	SendTime    string  `json:"send_time"`
	RawPayload  string  `json:"raw_payload"`
	CreatedTime string  `json:"created_at"`
	UnionID     string  `json:"unionid"`
}

type Fact struct {
	Source        Source     `json:"-"`
	SendTimeBasis string     `json:"send_time_basis"`
	SentAt        *time.Time `json:"sent_at"`
	CreatedAt     *time.Time `json:"created_at"`
}

const (
	SendTimeExplicitOffset = "explicit_offset"
	SendTimeCivilUnzoned   = "civil_unzoned"
	SendTimeUnmapped       = "unmapped"
)

type Result struct {
	Disposition Disposition     `json:"disposition"`
	Reason      string          `json:"reason"`
	Fact        *Fact           `json:"-"`
	Raw         json.RawMessage `json:"-"`
}

// AdaptHistory keeps order and cardinality. Repeated msgid values are not
// deduplicated: the source PK, not a guessed current archive identity, is id.
func AdaptHistory(payloads []json.RawMessage) []Result {
	rows := make([]Result, len(payloads))
	counts := map[int64]int{}
	for i, payload := range payloads {
		rows[i] = AdaptMessage(payload)
		if rows[i].Fact != nil {
			counts[rows[i].Fact.Source.ID]++
		}
	}
	for i := range rows {
		if rows[i].Fact != nil && counts[rows[i].Fact.Source.ID] > 1 {
			rows[i].Disposition, rows[i].Reason = Invalid, "message_source_id_duplicate"
		}
	}
	return rows
}

func AdaptMessage(payload json.RawMessage) Result {
	r := Result{Disposition: Invalid, Reason: "message_json_invalid", Raw: bytes.Clone(payload)}
	var fields map[string]json.RawMessage
	if !utf8.Valid(payload) || json.Unmarshal(payload, &fields) != nil || fields == nil {
		return r
	}
	for _, key := range strings.Fields(requiredFields + " " + nullableFields) {
		if _, ok := fields[key]; !ok {
			return r
		}
	}
	for _, key := range strings.Fields(requiredFields) {
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return r
		}
	}
	var source Source
	if json.Unmarshal(payload, &source) != nil {
		return r
	}
	r.Fact = &Fact{Source: source, SendTimeBasis: SendTimeUnmapped, SentAt: explicitTime(source.SendTime), CreatedAt: explicitTime(source.CreatedTime)}
	if r.Fact.SentAt != nil {
		r.Fact.SendTimeBasis = SendTimeExplicitOffset
	} else if validCivilTime(source.SendTime) {
		r.Fact.SendTimeBasis = SendTimeCivilUnzoned
	}
	if source.ID < 1 {
		r.Reason = "message_source_id_invalid"
		return r
	}
	r.Disposition, r.Reason = Candidate, ""
	if r.Fact.CreatedAt == nil {
		r.Disposition, r.Reason = Pending, "message_created_time_unmapped"
	} else if r.Fact.SendTimeBasis == SendTimeUnmapped {
		r.Disposition, r.Reason = Pending, "message_send_time_unmapped"
	}
	return r
}

// The V1 SDK emitted this civil clock string without its original timezone.
// Parse validates calendar fields only: the parsed value is never returned or
// assigned to SentAt. Exact round-trip rejects extra fractions or padding.
func validCivilTime(value string) bool {
	const layout = "2006-01-02 15:04:05"
	parsed, err := time.Parse(layout, value)
	return err == nil && parsed.Year() > 0 && parsed.Format(layout) == value
}

// No local timezone, numeric epoch unit, or timezone abbreviation is inferred.
// The original timestamp string remains in Source even when parsing succeeds.
func explicitTime(value string) *time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999Z07",
		"2006-01-02 15:04:05.999999999Z0700",
	} {
		if parsed, err := time.Parse(layout, value); err == nil && !parsed.IsZero() {
			parsed = parsed.UTC().Truncate(time.Microsecond)
			return &parsed
		}
	}
	return nil
}

// Summary contains counts only; arbitrary source strings never become keys.
type Summary struct {
	Rows, Decoded, Candidates, Pending, Invalid                      int
	Private, Group, OtherChatType, Text, Image, OtherMessageType     int
	NullContent, EmptyContent, ControlContent, PaddedContent         int
	NullByteContent, NullByteSendTime, NullByteChatOrType            int
	OverNativeBodyLimit, OverSidebarBodyLimit                        int
	EmptyUnionID, MissingReceiver, MissingOwner, NullSeq             int
	DuplicateMessageIDGroups, DuplicateMessageIDExtraRows            int
	ParsedSendTime, CivilSendTime, UnmappedSendTime, NumericSendTime int
	InvalidPayloadJSON, ObjectPayload, PayloadRoomID, PayloadToList  int
	Reasons                                                          map[string]int
}

func Summarize(rows []Result) Summary {
	var stream StreamSummary
	for _, row := range rows {
		stream.Add(row)
	}
	return stream.Summary
}

// StreamSummary retains only counts and fixed-size source ID/message digests,
// never a Result, Fact, Raw payload, body, or original message identifier.
// Its retained memory grows with distinct identifiers, not total body bytes.
type StreamSummary struct {
	Summary    Summary
	sourceIDs  map[int64]sourceDisposition
	messageIDs map[[32]byte]bool
}

type sourceDisposition struct {
	disposition Disposition
	reason      string
	duplicate   bool
}

func (stream *StreamSummary) Add(row Result) {
	if stream.sourceIDs == nil {
		stream.sourceIDs = map[int64]sourceDisposition{}
		stream.messageIDs = map[[32]byte]bool{}
		stream.Summary.Reasons = map[string]int{}
	}
	s := &stream.Summary
	s.Rows++
	if row.Fact != nil {
		id := row.Fact.Source.ID
		if previous, found := stream.sourceIDs[id]; found {
			if !previous.duplicate {
				// A later duplicate also invalidates the first source row, just
				// as AdaptHistory does, without retaining that row's payload.
				s.addDisposition(previous.disposition, previous.reason, -1)
				s.addDisposition(Invalid, "message_source_id_duplicate", 1)
				stream.sourceIDs[id] = sourceDisposition{duplicate: true}
			}
			row.Disposition, row.Reason = Invalid, "message_source_id_duplicate"
		} else {
			stream.sourceIDs[id] = sourceDisposition{disposition: row.Disposition, reason: row.Reason}
		}
	}
	s.addDisposition(row.Disposition, row.Reason, 1)
	if row.Fact == nil {
		return
	}
	s.Decoded++
	f := row.Fact.Source
	messageID := sha256.Sum256([]byte(f.MessageID))
	if duplicate, found := stream.messageIDs[messageID]; found {
		if !duplicate {
			s.DuplicateMessageIDGroups++
			stream.messageIDs[messageID] = true
		}
		s.DuplicateMessageIDExtraRows++
	} else {
		stream.messageIDs[messageID] = false
	}
	switch f.ChatType {
	case "private":
		s.Private++
	case "group":
		s.Group++
	default:
		s.OtherChatType++
	}
	switch f.MessageType {
	case "text":
		s.Text++
	case "image":
		s.Image++
	default:
		s.OtherMessageType++
	}
	if f.Content == nil {
		s.NullContent++
	} else {
		v := *f.Content
		if strings.IndexByte(v, 0) >= 0 {
			s.NullByteContent++
		}
		if v == "" {
			s.EmptyContent++
		}
		if strings.IndexFunc(v, unicode.IsControl) >= 0 {
			s.ControlContent++
		}
		if strings.TrimSpace(v) != v {
			s.PaddedContent++
		}
		if len(v) > 20000 {
			s.OverNativeBodyLimit++
		}
		if utf8.RuneCountInString(v) > 10000 {
			s.OverSidebarBodyLimit++
		}
	}
	if f.UnionID == "" {
		s.EmptyUnionID++
	}
	if f.Receiver == nil || *f.Receiver == "" {
		s.MissingReceiver++
	}
	if f.OwnerUserID == nil || *f.OwnerUserID == "" {
		s.MissingOwner++
	}
	if f.Seq == nil {
		s.NullSeq++
	}
	if strings.IndexByte(f.SendTime, 0) >= 0 {
		s.NullByteSendTime++
	}
	if strings.IndexByte(f.ChatType, 0) >= 0 || strings.IndexByte(f.MessageType, 0) >= 0 {
		s.NullByteChatOrType++
	}
	switch row.Fact.SendTimeBasis {
	case SendTimeExplicitOffset:
		s.ParsedSendTime++
	case SendTimeCivilUnzoned:
		s.CivilSendTime++
	default:
		s.UnmappedSendTime++
	}
	if f.SendTime != "" && strings.IndexFunc(f.SendTime, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
		s.NumericSendTime++
	}
	if !json.Valid([]byte(f.RawPayload)) {
		s.InvalidPayloadJSON++
	} else {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(f.RawPayload), &object) == nil && object != nil {
			s.ObjectPayload++
			if _, ok := object["roomid"]; ok {
				s.PayloadRoomID++
			}
			if _, ok := object["tolist"]; ok {
				s.PayloadToList++
			}
		}
	}
}

func (s *Summary) addDisposition(disposition Disposition, reason string, delta int) {
	switch disposition {
	case Candidate:
		s.Candidates += delta
	case Pending:
		s.Pending += delta
	case Invalid:
		s.Invalid += delta
	}
	if reason != "" {
		s.Reasons[reason] += delta
		if s.Reasons[reason] == 0 {
			delete(s.Reasons, reason)
		}
	}
}
