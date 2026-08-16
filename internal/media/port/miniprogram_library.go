package port

import "time"

// MiniProgramCard is the persisted local card metadata. Description and Tags
// are deliberately absent: the frozen legacy endpoint accepts them but never
// persists or returns them for mini-program cards.
type MiniProgramCard struct {
	ID                    int64
	Name                  string
	AppID                 string
	PagePath              string
	Title                 string
	ThumbImageID          *int64
	ThumbMediaID          string
	ThumbMediaIDExpiresAt string
	ThumbImageURL         string
	ThumbImageBase64      string
	Enabled               bool
	CreatedBy             int64
	UpdatedBy             int64
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type MiniProgramListQuery struct {
	Limit, Offset int32
	EnabledOnly   bool
	Search        string
}

type MiniProgramPage struct {
	Items  []MiniProgramCard
	Total  int64
	Limit  int32
	Offset int32
}

// MiniProgramUpsert preserves request presence. The compatibility adapter
// resolves pagepath/page_path and appid/app_id aliases before constructing it.
// Description and Tags remain input-only legacy fields by design.
type MiniProgramUpsert struct {
	Name, Title, AppID, PagePath, ThumbMediaID *string
	ThumbImageID                               *int64
	Enabled                                    *bool
	ResolveThumbMedia                          *bool
	Description                                *string
	Tags                                       *[]string
}

type MiniProgramCreateCommand struct {
	MiniProgramUpsert
	Actor          int64
	IdempotencyKey string
}

type MiniProgramUpdateCommand struct {
	ID int64
	MiniProgramUpsert
	Actor          int64
	IdempotencyKey string
}

type MiniProgramDeleteCommand struct {
	ID             int64
	Actor          int64
	IdempotencyKey string
}

type MiniProgramResolveCommand struct {
	ID             int64
	Actor          int64
	IdempotencyKey string
}

// MiniProgramThumbResolution describes only thumbnail-media cache resolution.
// It is intentionally not evidence that a card is sendable or that an AppID
// and pagepath can be reached by the external mini-program platform.
type MiniProgramThumbResolution struct {
	OK                       bool
	ThumbMediaID             string
	Source                   string
	AdapterMode              string
	Error                    string
	ErrorMessage             string
	ThumbImageID             *int64
	SideEffectExecuted       bool
	RealExternalCallExecuted bool
}

type MiniProgramMutationResult struct {
	Item         MiniProgramCard
	ThumbResolve *MiniProgramThumbResolution
}

type MiniProgramDeleteResult struct {
	ID          int64
	Deleted     bool
	HardDeleted bool
}
