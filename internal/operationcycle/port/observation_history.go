package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrCycleObservationInvalid     = errors.New("invalid cycle observation history")
	ErrCycleObservationConflict    = errors.New("cycle observation history conflict")
	ErrCycleObservationUnavailable = errors.New("cycle observation history unavailable")
)

// These immutable source observations never identify an executable V2 run.
type HistoricalCycleMetric struct {
	ID                   int64           `json:"id"`
	SourceID             int64           `json:"source_id"`
	SourceKeyDigest      [32]byte        `json:"-"`
	SourcePayloadDigest  [32]byte        `json:"-"`
	SourceFieldDigest    [32]byte        `json:"-"`
	RunSourceID          int64           `json:"run_source_id"`
	MetricKey            string          `json:"metric_key"`
	Label                string          `json:"label"`
	Numerator            *float64        `json:"numerator"`
	Denominator          *float64        `json:"denominator"`
	Value                *float64        `json:"value"`
	Unit                 string          `json:"unit"`
	ObservationWindow    string          `json:"observation_window"`
	DataSource           string          `json:"data_source"`
	DataQuality          string          `json:"data_quality"`
	LimitationsJSON      json.RawMessage `json:"limitations"`
	IsCausal             bool            `json:"is_causal"`
	ValueStatus          string          `json:"value_status"`
	LastSnapshotSourceID int64           `json:"last_snapshot_source_id"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// Href remains private evidence; readers must not expose it as an active link.
type HistoricalCycleReference struct {
	ID                   int64     `json:"id"`
	SourceID             int64     `json:"source_id"`
	SourceKeyDigest      [32]byte  `json:"-"`
	SourcePayloadDigest  [32]byte  `json:"-"`
	SourceFieldDigest    [32]byte  `json:"-"`
	RunSourceID          int64     `json:"run_source_id"`
	ReferenceKey         string    `json:"reference_key"`
	ReferenceType        string    `json:"reference_type"`
	Label                string    `json:"label"`
	SourceSystem         string    `json:"source_system"`
	ReferenceSourceID    string    `json:"reference_source_id"`
	Href                 string    `json:"-"`
	EvidenceHash         string    `json:"evidence_hash"`
	DataStatus           string    `json:"data_status"`
	LastSnapshotSourceID int64     `json:"last_snapshot_source_id"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type CycleObservationReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type CycleObservationQuery struct{ Limit, Offset int32 }

// The store and journal writes must share the caller's UnitOfWork transaction.
type CycleObservationStore interface {
	CreateHistoricalCycleMetric(context.Context, HistoricalCycleMetric) (HistoricalCycleMetric, error)
	GetHistoricalCycleMetric(context.Context, int64) (HistoricalCycleMetric, error)
	CreateHistoricalCycleReference(context.Context, HistoricalCycleReference) (HistoricalCycleReference, error)
	GetHistoricalCycleReference(context.Context, int64) (HistoricalCycleReference, error)
}

type CycleObservationJournal interface {
	LoadCycleObservation(context.Context, string, string) (CycleObservationReceipt, bool, error)
	RecordCycleObservation(context.Context, CycleObservationReceipt) error
}

type CycleObservationReader interface {
	GetHistoricalCycleMetric(context.Context, int64) (HistoricalCycleMetric, error)
	ListHistoricalCycleMetric(context.Context, CycleObservationQuery) ([]HistoricalCycleMetric, int64, error)
	GetHistoricalCycleReference(context.Context, int64) (HistoricalCycleReference, error)
	ListHistoricalCycleReference(context.Context, CycleObservationQuery) ([]HistoricalCycleReference, int64, error)
}
