package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrSourceSchemaDrift = errors.New("DM01 source schema drift")

// SourceReader is the closed application port for a legacy DM01 snapshot.
// Implementations own connection and transaction details; callers cannot
// provide SQL, table names, or database handles.
type SourceReader interface {
	WithSnapshot(context.Context, Manifest, func(SourceSnapshot) error) error
	Close()
}

type SourceSnapshot interface {
	Bounds() []SourceUpperBound
	EachOwnerRoleMap(context.Context, SourceUpperBound, func(OwnerRoleMapRow) error) error
	EachCustomerIdentity(context.Context, SourceUpperBound, func(CustomerIdentityRow) error) error
	EachExternalIdentityMap(context.Context, SourceUpperBound, func(ExternalIdentityMapRow) error) error
}

type SourceUpperBound struct {
	Table     string
	Watermark time.Time
	SourceKey string
	Empty     bool
}

type SourceColumn struct {
	Ordinal  int32
	Name     string
	DataType string
	NotNull  bool
}

// CanonicalSchemaDigest binds ordered physical column identity, type, and
// nullability. The adapter must supply pg_catalog rows in ordinal order.
func CanonicalSchemaDigest(columns []SourceColumn) (string, error) {
	if len(columns) == 0 {
		return "", ErrSourceSchemaDrift
	}
	var canonical strings.Builder
	var previousOrdinal int32
	for _, column := range columns {
		if column.Ordinal <= previousOrdinal || strings.TrimSpace(column.Name) != column.Name || column.Name == "" || strings.TrimSpace(column.DataType) != column.DataType || column.DataType == "" || strings.ContainsAny(column.Name+column.DataType, "\x1e\x1f") {
			return "", ErrSourceSchemaDrift
		}
		previousOrdinal = column.Ordinal
		canonical.WriteString(strconv.FormatInt(int64(column.Ordinal), 10))
		canonical.WriteByte('\x1f')
		canonical.WriteString(column.Name)
		canonical.WriteByte('\x1f')
		canonical.WriteString(column.DataType)
		canonical.WriteByte('\x1f')
		canonical.WriteString(strconv.FormatBool(column.NotNull))
		canonical.WriteByte('\x1e')
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(digest[:]), nil
}

type OwnerRoleMapRow struct {
	UserID      string
	DisplayName string
	Active      bool
	UpdatedAt   time.Time
	Payload     json.RawMessage
}

type CustomerIdentityRow struct {
	UnionID          string
	CustomerName     string
	AvatarURL        string
	Gender           *int16
	PrimaryOwnerUser string
	FirstSeenAt      time.Time
	LastSeenAt       time.Time
	UpdatedAt        time.Time
	Payload          json.RawMessage
}

type ExternalIdentityMapRow struct {
	ID             int64
	ExternalUserID string
	UnionID        string
	CorpID         string
	UpdatedAt      time.Time
	Payload        json.RawMessage
}
