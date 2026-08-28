// Package v1radarhistory parses archived V1 Radar click rows into inert
// historical facts. It has no current Radar store, event, queue, or Provider
// dependency.
package v1radarhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const ClickTableID = "public/radar_click_events"

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest binds a source field without making the field recoverable from
// a candidate. It is not a V2 identity or a Provider receipt.
type OpaqueDigest [sha256.Size]byte

// ClickSource stays private to the migration flow. UnionID may only be
// resolved later through an explicit DM01 verification; it is never a V2
// customer ID and is excluded from JSON output.
type ClickSource struct {
	UnionID string
}

// SensitiveDigests preserve evidence for fields V2 must not place into its
// current Radar tracking projection.
type SensitiveDigests struct {
	OpenID           OpaqueDigest `json:"openid_digest"`
	ExternalUserID   OpaqueDigest `json:"external_userid_digest"`
	CampaignID       OpaqueDigest `json:"campaign_id_digest"`
	StaffID          OpaqueDigest `json:"staff_id_digest"`
	UserAgent        OpaqueDigest `json:"user_agent_digest"`
	IP               OpaqueDigest `json:"ip_digest"`
	PersonID         OpaqueDigest `json:"person_id_digest"`
	IPHash           OpaqueDigest `json:"ip_hash_digest"`
	CampaignSnapshot OpaqueDigest `json:"campaign_snapshot_digest"`
	StaffSnapshot    OpaqueDigest `json:"staff_snapshot_digest"`
	Referer          OpaqueDigest `json:"referer_digest"`
	QueryParams      OpaqueDigest `json:"query_params_digest"`
}

// ClickFact is a non-executable source fact. LinkSourceID is deliberately not
// translated into a V2 radar_links ID; a future owner-owned crosswalk must do
// that separately.
type ClickFact struct {
	SourceID              int64            `json:"source_id"`
	LinkSourceID          int64            `json:"link_source_id"`
	Code                  string           `json:"code"`
	RawStage              string           `json:"raw_stage"`
	SourceChannel         string           `json:"source_channel"`
	TargetTypeSnapshot    string           `json:"target_type_snapshot"`
	SourceChannelSnapshot string           `json:"source_channel_snapshot"`
	ErrorCode             string           `json:"error_code"`
	CreatedAt             time.Time        `json:"created_at"`
	Sensitive             SensitiveDigests `json:"sensitive_digests"`
	Source                ClickSource      `json:"-"`
}

type Result struct {
	SourceID    int64       `json:"source_id"`
	Disposition Disposition `json:"disposition"`
	Reason      string      `json:"reason,omitempty"`
	Fact        *ClickFact  `json:"fact,omitempty"`
}

type clickJSON struct {
	ID                    int64           `json:"id"`
	LinkID                int64           `json:"link_id"`
	Code                  string          `json:"code"`
	Stage                 string          `json:"stage"`
	OpenID                string          `json:"openid"`
	UnionID               string          `json:"unionid"`
	ExternalUserID        string          `json:"external_userid"`
	SourceChannel         string          `json:"source_channel"`
	CampaignID            string          `json:"campaign_id"`
	StaffID               string          `json:"staff_id"`
	UserAgent             string          `json:"user_agent"`
	IP                    string          `json:"ip"`
	CreatedAt             time.Time       `json:"created_at"`
	TargetTypeSnapshot    string          `json:"target_type_snapshot"`
	PersonID              string          `json:"person_id"`
	IPHash                string          `json:"ip_hash"`
	SourceChannelSnapshot string          `json:"source_channel_snapshot"`
	CampaignIDSnapshot    string          `json:"campaign_id_snapshot"`
	StaffIDSnapshot       string          `json:"staff_id_snapshot"`
	Referer               string          `json:"referer"`
	QueryParams           json.RawMessage `json:"query_params_json"`
	ErrorCode             string          `json:"error_code"`
}

const clickRequiredFields = "id link_id code stage openid unionid external_userid source_channel campaign_id staff_id user_agent ip created_at target_type_snapshot person_id ip_hash source_channel_snapshot campaign_id_snapshot staff_id_snapshot referer query_params_json error_code"

// AdaptClicks preserves source order and count. It never creates a current
// tracking event or turns a raw stage into the V2 stage enum.
func AdaptClicks(rows []json.RawMessage) []Result {
	result := make([]Result, len(rows))
	for index, row := range rows {
		result[index] = AdaptClick(row)
	}
	quarantineDuplicateSourceIDs(result)
	return result
}

func AdaptClick(payload json.RawMessage) Result {
	fields, ok := clickObject(payload)
	if !ok {
		return quarantine(0, "radar_click_json_invalid")
	}
	sourceID := clickSourceID(fields)
	var source clickJSON
	if !decodeClick(fields, payload, &source, clickRequiredFields) || source.ID < 1 || source.LinkID < 1 || source.CreatedAt.IsZero() {
		return quarantine(sourceID, "radar_click_shape_invalid")
	}
	digests, ok := clickSensitiveDigests(fields)
	if !ok {
		return quarantine(source.ID, "radar_click_shape_invalid")
	}
	return candidate(ClickFact{
		SourceID: source.ID, LinkSourceID: source.LinkID, Code: source.Code, RawStage: source.Stage,
		SourceChannel: source.SourceChannel, TargetTypeSnapshot: source.TargetTypeSnapshot,
		SourceChannelSnapshot: source.SourceChannelSnapshot, ErrorCode: source.ErrorCode,
		CreatedAt: source.CreatedAt, Sensitive: digests, Source: ClickSource{UnionID: source.UnionID},
	})
}

func candidate(fact ClickFact) Result {
	return Result{SourceID: fact.SourceID, Disposition: DispositionCandidate, Fact: &fact}
}

func quarantine(sourceID int64, reason string) Result {
	return Result{SourceID: sourceID, Disposition: DispositionQuarantine, Reason: reason}
}

func quarantineDuplicateSourceIDs(rows []Result) {
	counts := make(map[int64]int, len(rows))
	for _, row := range rows {
		if row.Disposition == DispositionCandidate && row.Fact != nil {
			counts[row.SourceID]++
		}
	}
	for index := range rows {
		if rows[index].Disposition == DispositionCandidate && counts[rows[index].SourceID] > 1 {
			rows[index] = quarantine(rows[index].SourceID, "radar_click_source_ambiguous")
		}
	}
}

func clickSensitiveDigests(source clickFields) (SensitiveDigests, bool) {
	values := make([]OpaqueDigest, 0, 12)
	for _, name := range []string{"openid", "external_userid", "campaign_id", "staff_id", "user_agent", "ip", "person_id", "ip_hash", "campaign_id_snapshot", "staff_id_snapshot", "referer", "query_params_json"} {
		digest, ok := clickFieldDigest(source, name)
		if !ok {
			return SensitiveDigests{}, false
		}
		values = append(values, digest)
	}
	return SensitiveDigests{OpenID: values[0], ExternalUserID: values[1], CampaignID: values[2], StaffID: values[3], UserAgent: values[4], IP: values[5], PersonID: values[6], IPHash: values[7], CampaignSnapshot: values[8], StaffSnapshot: values[9], Referer: values[10], QueryParams: values[11]}, true
}

func clickFieldDigest(source clickFields, name string) (OpaqueDigest, bool) {
	raw, found := source[name]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || !json.Valid(raw) {
		return OpaqueDigest{}, false
	}
	sum := sha256.Sum256(append(append([]byte("v1-radar-history-field-v1\x00"), name...), raw...))
	return OpaqueDigest(sum), true
}

type clickFields map[string]json.RawMessage

func clickObject(value json.RawMessage) (clickFields, bool) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	fields := make(clickFields)
	if decoder.Decode(&fields) != nil || fields == nil {
		return nil, false
	}
	var extra any
	return fields, errors.Is(decoder.Decode(&extra), io.EOF)
}

func decodeClick(fields clickFields, payload json.RawMessage, target any, required string) bool {
	for _, name := range bytes.Fields([]byte(required)) {
		raw, found := fields[string(name)]
		if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func clickSourceID(fields clickFields) int64 {
	raw, found := fields["id"]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0
	}
	var id int64
	if json.Unmarshal(raw, &id) != nil || id < 1 {
		return 0
	}
	return id
}
