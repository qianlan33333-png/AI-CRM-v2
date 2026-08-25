package port

import (
	"context"
	"time"
)

type EventStage string

const (
	EventStageLanding             EventStage = "landing"
	EventStageRedirect            EventStage = "redirect"
	EventStageViewerOpen          EventStage = "viewer_open"
	EventStageImageLoaded         EventStage = "image_loaded"
	EventStagePDFOpened           EventStage = "pdf_opened"
	EventStagePDFManifestLoaded   EventStage = "pdf_manifest_loaded"
	EventStagePDFPageLoaded       EventStage = "pdf_page_loaded"
	EventStagePDFPageError        EventStage = "pdf_page_error"
	EventStageImageManifestLoaded EventStage = "image_manifest_loaded"
	EventStageImageVariantLoaded  EventStage = "image_variant_loaded"
)

func (stage EventStage) Valid() bool {
	switch stage {
	case EventStageLanding, EventStageRedirect, EventStageViewerOpen,
		EventStageImageLoaded, EventStagePDFOpened, EventStagePDFManifestLoaded,
		EventStagePDFPageLoaded, EventStagePDFPageError,
		EventStageImageManifestLoaded, EventStageImageVariantLoaded:
		return true
	default:
		return false
	}
}

func (stage EventStage) PublicRecordable() bool {
	switch stage {
	case EventStageViewerOpen, EventStageImageLoaded, EventStagePDFOpened,
		EventStagePDFManifestLoaded, EventStagePDFPageLoaded, EventStagePDFPageError,
		EventStageImageManifestLoaded, EventStageImageVariantLoaded:
		return true
	default:
		return false
	}
}

const (
	DefaultEventLimit      int32 = 100
	MaximumEventLimit      int32 = 500
	MaximumEventOffset     int32 = 1_000_000
	MaximumEventPage       int32 = 100_000
	MaximumEventExtraJSON        = 16 << 10
	MaximumPublicCodeBytes       = 64
	MaximumSidebarLimit    int32 = 500
)

type EventSource string

const (
	EventSourcePublicRedirect EventSource = "public_redirect"
	EventSourcePublicEvent    EventSource = "public_event"
)

type Event struct {
	EventID   int64       `json:"event_id"`
	ReceiptID string      `json:"receipt_id"`
	LinkID    LinkID      `json:"link_id"`
	Stage     EventStage  `json:"stage"`
	Page      *int32      `json:"page,omitempty"`
	Source    EventSource `json:"source"`
	CreatedAt time.Time   `json:"created_at"`
}

type EventReceipt struct {
	EventID                  int64     `json:"event_id"`
	ReceiptID                string    `json:"receipt_id"`
	CreatedAt                time.Time `json:"created_at"`
	Replayed                 bool      `json:"replayed"`
	LocalReceipt             bool      `json:"local_receipt"`
	IdentityAttributed       bool      `json:"identity_attributed"`
	RealExternalCallExecuted bool      `json:"real_external_call_executed"`
}

type PublicRedirect struct {
	DestinationURL string       `json:"destination_url"`
	Receipt        EventReceipt `json:"receipt"`
}

type RecordEventCommand struct {
	PublicCode     string
	Stage          EventStage
	Page           *int32
	Extra          map[string]any
	IdempotencyKey string
}

type EventListInput struct {
	LinkID LinkID
	Stage  *EventStage
	Start  *time.Time
	End    *time.Time
	Limit  int32
	Offset int32
}

type EventPage struct {
	Items                    []Event `json:"items"`
	Events                   []Event `json:"events"`
	Total                    int64   `json:"total"`
	Limit                    int32   `json:"limit"`
	Offset                   int32   `json:"offset"`
	HasMore                  bool    `json:"has_more"`
	IdentityAttributed       bool    `json:"identity_attributed"`
	RealExternalCallExecuted bool    `json:"real_external_call_executed"`
}

type EventStats struct {
	LinkID                   LinkID     `json:"link_id"`
	TotalEvents              int64      `json:"total_events"`
	TotalClicks              int64      `json:"total_clicks"`
	TotalLandings            int64      `json:"total_landings"`
	AuthorizedClicks         int64      `json:"authorized_clicks"`
	UniqueUsers              int64      `json:"unique_users"`
	AuthorizedUsers          int64      `json:"authorized_users"`
	Redirects                int64      `json:"redirects"`
	ViewerOpens              int64      `json:"viewer_opens"`
	ViewOpens                int64      `json:"view_opens"`
	ImageLoaded              int64      `json:"image_loaded"`
	PDFOpened                int64      `json:"pdf_opened"`
	TodayClicks              int64      `json:"today_clicks"`
	TodayLandings            int64      `json:"today_landings"`
	LastClickedAt            *time.Time `json:"last_clicked_at"`
	LastEventAt              *time.Time `json:"last_event_at"`
	LastViewedAt             *time.Time `json:"last_viewed_at"`
	IdentityAttributed       bool       `json:"identity_attributed"`
	RealExternalCallExecuted bool       `json:"real_external_call_executed"`
}

type SidebarLink struct {
	LinkID     LinkID    `json:"id"`
	Title      string    `json:"title"`
	TargetType string    `json:"target_type"`
	TypeLabel  string    `json:"type_label"`
	URL        string    `json:"url"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SidebarPage struct {
	Items                    []SidebarLink `json:"items"`
	Total                    int64         `json:"total"`
	Limit                    int32         `json:"limit"`
	Offset                   int32         `json:"offset"`
	LocalProjection          bool          `json:"local_projection"`
	IdentityAttributed       bool          `json:"identity_attributed"`
	RealExternalCallExecuted bool          `json:"real_external_call_executed"`
}

type InsertEventRecord struct {
	ReceiptID     string
	LinkID        LinkID
	Stage         EventStage
	Page          *int32
	Source        EventSource
	KeyDigest     []byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type EventStatsRecord struct {
	TotalEvents, TotalLandings, Redirects, ViewerOpens, ImageLoaded, PDFOpened, TodayLandings int64
	LastClickedAt, LastEventAt, LastViewedAt                                                  *time.Time
}

type TrackingApplication interface {
	ResolvePublicRedirect(context.Context, string) (PublicRedirect, error)
	RecordPublicEvent(context.Context, RecordEventCommand) (EventReceipt, error)
	ListEvents(context.Context, EventListInput) (EventPage, error)
	EventStats(context.Context, LinkID) (EventStats, error)
	SidebarLinks(context.Context, int32, int32, string) (SidebarPage, error)
}

type TrackingRepository interface {
	GetEnabledByCode(context.Context, string) (Link, error)
	InsertEvent(context.Context, InsertEventRecord) (Event, bool, error)
	GetEventByKey(context.Context, LinkID, []byte) (Event, [32]byte, error)
	ListEvents(context.Context, EventListInput) ([]Event, int64, error)
	EventStats(context.Context, LinkID) (EventStatsRecord, error)
	ListEnabledForSidebar(context.Context, int32, int32) ([]SidebarLink, int64, error)
}
