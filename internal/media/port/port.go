package port

import (
	"context"
	"time"
)

type Image struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	FileName    string    `json:"file_name"`
	FileSize    int32     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
	Width       int32     `json:"width"`
	Height      int32     `json:"height"`
	Description string    `json:"description"`
	Tags        string    `json:"tags"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ImageFacets struct {
	Categories []string
	Tags       []string
}

type UploadCommand struct {
	Actor          int64
	IdempotencyKey string
	FileName       string
	DeclaredType   string
	Content        []byte
	Name           string
	Description    string
	Tags           string
	Category       string
}

type GroupInvite struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	JoinURL      string     `json:"join_url"`
	CoverImageID int64      `json:"cover_image_id,omitempty"`
	Enabled      bool       `json:"enabled"`
	CreatedBy    int64      `json:"created_by"`
	UpdatedBy    int64      `json:"updated_by"`
	Version      int64      `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
}

type GroupInviteCreateCommand struct {
	Name, Title, Description, JoinURL string
	CoverImageID                      int64
	Enabled                           *bool
	Actor                             int64
	IdempotencyKey                    string
}

type GroupInvitePatch struct {
	Name, Title, Description, JoinURL *string
	CoverImageID                      *int64
	Enabled                           *bool
}

type GroupInviteUpdateCommand struct {
	ID int64
	GroupInvitePatch
	Actor          int64
	IdempotencyKey string
}

type GroupInviteArchiveCommand struct {
	ID             int64
	Actor          int64
	IdempotencyKey string
}

type GroupInviteListQuery struct {
	Limit, Offset int32
	EnabledOnly   bool
	Search        string
}

type GroupInvitePage struct {
	Items  []GroupInvite `json:"items"`
	Total  int64         `json:"total"`
	Limit  int32         `json:"limit"`
	Offset int32         `json:"offset"`
}

type ImageMetadataReader interface {
	ImageExists(context.Context, int64) (bool, error)
}
