// Package port defines the release-plane boundary. It deliberately accepts
// attestations only; it never invokes deployment, backup, or providers.
package port

import (
	"context"
	"time"
)

type CandidateState string

const (
	CandidateDraft           CandidateState = "draft"
	CandidatePrepared        CandidateState = "prepared"
	CandidateCutoverActive   CandidateState = "cutover_active"
	CandidateActivated       CandidateState = "activated"
	CandidateRollbackPending CandidateState = "rollback_pending"
	CandidateRolledBack      CandidateState = "rolled_back"
)

type PrerequisiteKind string

const (
	PrerequisiteNightly            PrerequisiteKind = "nightly"
	PrerequisiteBackupRestoreDrill PrerequisiteKind = "backup_restore_drill"
	PrerequisiteMigration          PrerequisiteKind = "migration"
	PrerequisiteContactClosure     PrerequisiteKind = "contact_closure"
	PrerequisiteCampaignClosure    PrerequisiteKind = "campaign_closure"
	PrerequisiteOutboundClosure    PrerequisiteKind = "outbound_closure"
	PrerequisiteCommerceClosure    PrerequisiteKind = "commerce_closure"
)

type CutoverStep string

const (
	CutoverAnnounce     CutoverStep = "announce"
	CutoverQuiesce      CutoverStep = "quiesce"
	CutoverSchemaVerify CutoverStep = "schema_verify"
	CutoverSwitch       CutoverStep = "switch"
	CutoverVerify       CutoverStep = "verify"
)

var FixedCutoverSteps = []CutoverStep{CutoverAnnounce, CutoverQuiesce, CutoverSchemaVerify, CutoverSwitch, CutoverVerify}

type Candidate struct {
	ID                  int64
	CommitSHA           string
	ArtifactDigest      string
	ManifestDigest      string
	ConfigDigest        string
	TargetSchemaVersion int64
	State               CandidateState
	CreatedBy           int64
	CreatedAt           time.Time
	PreparedAt          *time.Time
	ActivatedAt         *time.Time
	RollbackRequestedAt *time.Time
	RolledBackAt        *time.Time
}

type PrerequisiteReceipt struct {
	ID          int64
	CandidateID int64
	Kind        PrerequisiteKind
	EvidenceSHA string
	RecordedBy  int64
	RecordedAt  time.Time
}

type Readiness struct {
	CandidateID int64
	Ready       bool
	Missing     []PrerequisiteKind
	CheckedAt   time.Time
}

type WorkerLease struct {
	CandidateID int64
	Generation  int64
	Fence       string
	StartedBy   int64
	StartedAt   time.Time
	Active      bool
}

type CutoverJournalEntry struct {
	ID          int64
	CandidateID int64
	Step        CutoverStep
	Fence       string
	CompletedBy int64
	CompletedAt time.Time
}

type OperationReceipt struct {
	ID            int64
	Action        string
	ActorID       int64
	KeyDigest     string
	PayloadDigest string
	State         string
	Result        []byte
}

type Repository interface {
	CreateCandidate(context.Context, Candidate) (Candidate, error)
	GetCandidate(context.Context, int64) (Candidate, error)
	ListCandidates(context.Context, int32) ([]Candidate, error)
	UpdateState(context.Context, int64, CandidateState, time.Time) (Candidate, error)
	CreatePrerequisite(context.Context, PrerequisiteReceipt) (PrerequisiteReceipt, error)
	ListPrerequisites(context.Context, int64) ([]PrerequisiteReceipt, error)
	StartWorker(context.Context, WorkerLease) (WorkerLease, error)
	GetWorker(context.Context, int64) (WorkerLease, error)
	AppendCutoverStep(context.Context, CutoverJournalEntry) (CutoverJournalEntry, error)
	ListCutoverSteps(context.Context, int64) ([]CutoverJournalEntry, error)
	ReserveOperationReceipt(context.Context, OperationReceipt) (OperationReceipt, bool, error)
	CompleteOperationReceipt(context.Context, int64, []byte) (OperationReceipt, error)
}
