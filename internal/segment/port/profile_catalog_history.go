package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrProfileCatalogHistoryInvalid     = errors.New("invalid profile catalogue history")
	ErrProfileCatalogHistoryConflict    = errors.New("profile catalogue history conflict")
	ErrProfileCatalogHistoryUnavailable = errors.New("profile catalogue history unavailable")
)

// These are non-executable V1 facts. Only *HistoryID fields reference these
// V2 history tables; every SourceID retains its old namespace and signed value.
type HistoricalProfileTemplate struct {
	ID                           int64     `json:"id"`
	SourceID                     int64     `json:"source_id"`
	SourceKeyDigest              [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest          [32]byte  `json:"source_payload_digest"`
	TemplateCode                 string    `json:"template_code"`
	TemplateName                 string    `json:"template_name"`
	QuestionnaireSourceID        *int64    `json:"questionnaire_source_id"`
	SegmentationQuestionSourceID *int64    `json:"segmentation_question_source_id"`
	ProgramSourceID              *int64    `json:"program_source_id"`
	Description                  string    `json:"description"`
	OriginalEnabled              bool      `json:"original_enabled"`
	Version                      int64     `json:"version"`
	CreatedByDigest              [32]byte  `json:"created_by_digest"`
	UpdatedByDigest              [32]byte  `json:"updated_by_digest"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}
type HistoricalProfileCategory struct {
	ID                  int64     `json:"id"`
	SourceID            int64     `json:"source_id"`
	SourceKeyDigest     [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
	TemplateSourceID    int64     `json:"template_source_id"`
	TemplateHistoryID   int64     `json:"template_history_id"`
	CategoryKey         string    `json:"category_key"`
	CategoryName        string    `json:"category_name"`
	Description         string    `json:"description"`
	SortOrder           int64     `json:"sort_order"`
	OriginalEnabled     bool      `json:"original_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}
type HistoricalProfileOptionMapping struct {
	ID                  int64     `json:"id"`
	SourceID            int64     `json:"source_id"`
	SourceKeyDigest     [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
	TemplateSourceID    int64     `json:"template_source_id"`
	CategorySourceID    int64     `json:"category_source_id"`
	TemplateHistoryID   int64     `json:"template_history_id"`
	CategoryHistoryID   int64     `json:"category_history_id"`
	QuestionSourceID    int64     `json:"question_source_id"`
	OptionSourceID      int64     `json:"option_source_id"`
	CreatedAt           time.Time `json:"created_at"`
}
type ProfileCatalogHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type ProfileCatalogHistoryQuery struct {
	TemplateHistoryID, CategoryHistoryID *int64
	Limit, Offset                        int32
}

// Store and Journal share the caller transaction and have no runtime operations.
type ProfileCatalogHistoryStore interface {
	CreateHistoricalProfileTemplate(context.Context, HistoricalProfileTemplate) (HistoricalProfileTemplate, error)
	GetHistoricalProfileTemplate(context.Context, int64) (HistoricalProfileTemplate, error)
	CreateHistoricalProfileCategory(context.Context, HistoricalProfileCategory) (HistoricalProfileCategory, error)
	GetHistoricalProfileCategory(context.Context, int64) (HistoricalProfileCategory, error)
	CreateHistoricalProfileOptionMapping(context.Context, HistoricalProfileOptionMapping) (HistoricalProfileOptionMapping, error)
	GetHistoricalProfileOptionMapping(context.Context, int64) (HistoricalProfileOptionMapping, error)
}
type ProfileCatalogHistoryJournal interface {
	LoadProfileCatalogHistory(context.Context, string, string) (ProfileCatalogHistoryReceipt, bool, error)
	RecordProfileCatalogHistory(context.Context, ProfileCatalogHistoryReceipt) error
}
type ProfileCatalogHistoryReader interface {
	GetHistoricalProfileTemplate(context.Context, int64) (HistoricalProfileTemplate, error)
	ListHistoricalProfileTemplates(context.Context, ProfileCatalogHistoryQuery) ([]HistoricalProfileTemplate, int64, error)
	GetHistoricalProfileCategory(context.Context, int64) (HistoricalProfileCategory, error)
	ListHistoricalProfileCategories(context.Context, ProfileCatalogHistoryQuery) ([]HistoricalProfileCategory, int64, error)
	GetHistoricalProfileOptionMapping(context.Context, int64) (HistoricalProfileOptionMapping, error)
	ListHistoricalProfileOptionMappings(context.Context, ProfileCatalogHistoryQuery) ([]HistoricalProfileOptionMapping, int64, error)
}
