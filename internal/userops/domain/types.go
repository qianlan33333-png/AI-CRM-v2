// Package domain owns User Ops local facts. It deliberately contains no
// provider, outbound, external identity, or customer-table implementation.
package domain

import "time"

// CustomerID is always the local canonical customer identifier. It is never a
// unionid, phone number, openid, or provider identifier.
type CustomerID int64

func (id CustomerID) Valid() bool { return id > 0 }

type PlanID int64

func (id PlanID) Valid() bool { return id > 0 }

type SendRecordID int64

func (id SendRecordID) Valid() bool { return id > 0 }

// DoNotDisturb is a local operational preference. Reason is operator-facing
// local text only; it is not an external consent, provider flag, or delivery
// outcome.
type DoNotDisturb struct {
	CustomerID CustomerID `json:"customer_id"`
	Reason     string     `json:"reason"`
	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ContentInput is the caller-provided local message material. It intentionally
// supports only the material kinds evidenced by the current send-content
// contract: text, image, Mini Program, and private attachment references.
type ContentInput struct {
	Text                  string  `json:"text"`
	ImageLibraryIDs       []int64 `json:"image_library_ids"`
	MiniProgramLibraryIDs []int64 `json:"miniprogram_library_ids"`
	AttachmentLibraryIDs  []int64 `json:"attachment_library_ids"`
}

// ContentSnapshot is the normalized, immutable local content fact bound to a
// plan. It contains no URL, provider media ID, dynamic card, group invite, or
// external identifier.
type ContentSnapshot struct {
	Text                  string  `json:"text"`
	ImageLibraryIDs       []int64 `json:"image_library_ids"`
	MiniProgramLibraryIDs []int64 `json:"miniprogram_library_ids"`
	AttachmentLibraryIDs  []int64 `json:"attachment_library_ids"`
	ContentDigest         string  `json:"content_digest"`
}

type LocalPlanState string

const (
	LocalPlanDraft         LocalPlanState = "draft"
	LocalPlanPendingReview LocalPlanState = "pending_review"
)

func (state LocalPlanState) Valid() bool {
	return state == LocalPlanDraft || state == LocalPlanPendingReview
}

// LocalPlan only describes a local reviewable plan. It is never evidence that
// a message was enqueued, dispatched, received, or delivered.
type LocalPlan struct {
	ID           PlanID          `json:"plan_id,string"`
	State        LocalPlanState  `json:"state"`
	Content      ContentSnapshot `json:"content"`
	TargetDigest string          `json:"target_digest"`
	TargetCount  int32           `json:"target_count"`
	Version      int64           `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type SendTechnicalState string

const (
	SendTechnicalStateDraft         SendTechnicalState = "draft"
	SendTechnicalStatePendingReview SendTechnicalState = "pending_review"
	SendTechnicalStateNotDispatched SendTechnicalState = "not_dispatched"
)

func (state SendTechnicalState) Valid() bool {
	switch state {
	case SendTechnicalStateDraft, SendTechnicalStatePendingReview, SendTechnicalStateNotDispatched:
		return true
	default:
		return false
	}
}

// SendRecord is a local technical record attached to a local plan. It
// intentionally has no provider message ID, receipt, delivery timestamp, or
// external result.
type SendRecord struct {
	ID              SendRecordID       `json:"send_record_id,string"`
	PlanID          PlanID             `json:"plan_id,string"`
	CustomerID      CustomerID         `json:"customer_id"`
	TechnicalStatus SendTechnicalState `json:"technical_status"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}
