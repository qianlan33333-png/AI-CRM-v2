// Package port defines Admin Ops' local control-plane contract. It carries no
// credential material: secret-bearing operations use only a safe reference.
package port

import "time"

type CredentialKind string

const (
	CredentialDirectAPIKey CredentialKind = "direct_api_key"
	CredentialAPIClient    CredentialKind = "api_client"
)

type Credential struct {
	ID          int64
	Kind        CredentialKind
	ClientID    string
	DisplayName string
	State       string
	SecretRef   string
	SecretMask  string
	Metadata    []byte
	Version     int64
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Category struct {
	Key       string
	Enabled   bool
	Settings  []byte
	Version   int64
	UpdatedBy string
	UpdatedAt time.Time
}

type Release struct {
	ID                  int64
	State               string
	Changes             []byte
	Checksum            string
	BasedOnReleaseID    *int64
	RollbackOfReleaseID *int64
	CreatedBy           string
	PublishedBy         string
	CreatedAt           time.Time
	ValidatedAt         *time.Time
	PublishedAt         *time.Time
}

type Job struct {
	ID          int64
	Key         string
	Kind        string
	State       string
	TargetRef   string
	Request     []byte
	Result      []byte
	Version     int64
	RequestedBy string
	FailureCode string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	UpdatedAt   time.Time
}
