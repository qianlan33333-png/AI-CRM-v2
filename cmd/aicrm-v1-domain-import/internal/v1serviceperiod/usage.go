// Package v1serviceperiod classifies V1 service-period history without
// creating current products, entitlements, customers, or external effects.
package v1serviceperiod

import (
	"bytes"
	"encoding/json"
	"time"
)

type UsageSnapshotSource struct {
	Payload        json.RawMessage
	RedactedFields []string
}

type UsageSnapshotFact struct {
	HuangyoucanUserID string     `json:"huangyoucan_user_id"`
	UnionID           string     `json:"unionid"`
	MobileMD5         string     `json:"mobile_md5"`
	FormallyLoggedIn  bool       `json:"formally_logged_in"`
	HasTokenUsage     bool       `json:"has_token_usage"`
	LearningPlanID    string     `json:"learning_plan_id"`
	LearningCurrent   *int32     `json:"learning_plan_current"`
	LearningTotal     *int32     `json:"learning_plan_total"`
	OpenCount7d       int32      `json:"open_count_7d"`
	LastOpenAt        *time.Time `json:"last_open_at"`
	RefreshedAt       time.Time  `json:"refreshed_at"`
}

type UsageSnapshotDisposition string

const (
	UsageSnapshotCandidate UsageSnapshotDisposition = "historical_candidate"
	UsageSnapshotInvalid   UsageSnapshotDisposition = "invalid"
)

type UsageSnapshotResult struct {
	Disposition UsageSnapshotDisposition
	Reason      string
	Fact        *UsageSnapshotFact
}

var usageSnapshotFields = [...]string{
	"huangyoucan_user_id", "unionid", "mobile_md5", "formally_logged_in", "has_token_usage", "learning_plan_id",
	"learning_plan_current", "learning_plan_total", "open_count_7d", "last_open_at", "refreshed_at",
}

// AdaptUsageSnapshots is pure and count-conserving. An empty unionid and NULL
// nullable fields remain source facts; neither mobile_md5 nor any plan field
// is interpreted as a Customer, Product, entitlement, or current login state.
func AdaptUsageSnapshots(rows []UsageSnapshotSource) []UsageSnapshotResult {
	result := make([]UsageSnapshotResult, len(rows))
	for index, row := range rows {
		result[index] = adaptUsageSnapshot(row)
	}
	return result
}

func adaptUsageSnapshot(row UsageSnapshotSource) UsageSnapshotResult {
	if usageSnapshotRedacted(row.RedactedFields) {
		return UsageSnapshotResult{Disposition: UsageSnapshotInvalid, Reason: "usage_snapshot_redacted"}
	}
	var fact UsageSnapshotFact
	if !decodeUsageSnapshot(row.Payload, &fact) {
		return UsageSnapshotResult{Disposition: UsageSnapshotInvalid, Reason: "usage_snapshot_json_invalid"}
	}
	if fact.HuangyoucanUserID == "" || fact.OpenCount7d < 0 || fact.RefreshedAt.IsZero() ||
		fact.LastOpenAt != nil && fact.LastOpenAt.IsZero() {
		return UsageSnapshotResult{Disposition: UsageSnapshotInvalid, Reason: "usage_snapshot_shape_invalid"}
	}
	return UsageSnapshotResult{Disposition: UsageSnapshotCandidate, Fact: &fact}
}

func usageSnapshotRedacted(fields []string) bool {
	for _, field := range fields {
		for _, required := range usageSnapshotFields {
			if field == required {
				return true
			}
		}
	}
	return false
}

func decodeUsageSnapshot(payload json.RawMessage, target *UsageSnapshotFact) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return false
	}
	for _, name := range usageSnapshotFields {
		value, found := fields[name]
		if !found {
			return false
		}
		switch name {
		case "learning_plan_current", "learning_plan_total", "last_open_at":
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return json.Unmarshal(payload, target) == nil
}
