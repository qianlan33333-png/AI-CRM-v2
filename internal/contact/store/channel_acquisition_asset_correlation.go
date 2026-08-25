package store

import (
	"context"
	"encoding/base64"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

// ChannelAcquisitionAssetCorrelationRepository resolves only the exact
// CorpID+State tuple. It deliberately returns cardinality instead of choosing
// a current, latest, or first match.
type ChannelAcquisitionAssetCorrelationRepository struct{ pool *pgxpool.Pool }

var _ contactport.AcquisitionAssetCorrelationResolver = (*ChannelAcquisitionAssetCorrelationRepository)(nil)

func NewChannelAcquisitionAssetCorrelationRepository(pool *pgxpool.Pool) *ChannelAcquisitionAssetCorrelationRepository {
	return &ChannelAcquisitionAssetCorrelationRepository{pool: pool}
}

func (repository *ChannelAcquisitionAssetCorrelationRepository) ResolveAcquisitionAssetCorrelation(ctx context.Context, corpID, state string, occurredAt time.Time) (contactport.AcquisitionAssetCorrelationResolution, error) {
	if repository == nil || repository.pool == nil || ctx == nil || ctx.Err() != nil || !validCorrelationScope(corpID) || !validStoredCorrelationKey(state) || occurredAt.IsZero() {
		return contactport.AcquisitionAssetCorrelationResolution{}, contactport.ErrAcquisitionAssetCorrelationUnavailable
	}
	rows, err := contactdb.New(repository.pool).ResolveChannelAcquisitionAssetCorrelation(ctx, contactdb.ResolveChannelAcquisitionAssetCorrelationParams{CorpID: corpID, CorrelationKey: state, OccurredAt: pgtype.Timestamptz{Time: occurredAt.UTC(), Valid: true}})
	if err != nil {
		return contactport.AcquisitionAssetCorrelationResolution{}, contactport.ErrAcquisitionAssetCorrelationUnavailable
	}
	return channelAcquisitionCorrelationResolution(rows), nil
}

func channelAcquisitionCorrelationResolution(rows []contactdb.ResolveChannelAcquisitionAssetCorrelationRow) contactport.AcquisitionAssetCorrelationResolution {
	switch len(rows) {
	case 0:
		return contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationZero}
	case 1:
		row := rows[0]
		return contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: contactport.AcquisitionAssetCorrelationMatch{EffectID: channelAcquisitionFormatEffectID(row.EffectID), ChannelID: row.ChannelID, Kind: contactport.AcquisitionAssetKind(row.AssetKind), AssetVersion: row.AssetVersion}}
	default:
		return contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationMultiple}
	}
}

func validCorrelationScope(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validStoredCorrelationKey(value string) bool {
	if !strings.HasPrefix(value, "ch02_") || len(value) != 48 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "ch02_"))
	return err == nil && len(raw) == 32
}
