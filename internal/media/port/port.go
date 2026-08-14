package port

import "time"

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
