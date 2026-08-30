package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcsource "github.com/qianlan33333-png/AI-CRM-v2/internal/hxcsource/store/generated"
)

type MySQLCurrentSource struct{ connector driver.Connector }

func NewMySQLCurrentSource(dsn string) (*MySQLCurrentSource, error) {
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.MultiStatements {
		return nil, errors.New("invalid hxc mysql dsn")
	}
	config.ParseTime = true
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return nil, errors.New("invalid hxc mysql connector")
	}
	return &MySQLCurrentSource{connector: connector}, nil
}

func (source *MySQLCurrentSource) ReadCurrent(ctx context.Context) ([]hxcport.SourceCurrent, error) {
	if source == nil || source.connector == nil || ctx == nil {
		return nil, errors.New("invalid hxc mysql source")
	}
	database := sql.OpenDB(source.connector)
	defer database.Close()
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin hxc source read: %w", err)
	}
	defer tx.Rollback()
	rows, err := hxcsource.New(tx).ReadHXCCurrentSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("read hxc current source: %w", err)
	}
	result := make([]hxcport.SourceCurrent, 0, len(rows))
	for _, row := range rows {
		unionID, err := sourceNullString(row.Unionid)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source unionid: %w", err)
		}
		phone, err := sourceNullString(row.Phone)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source phone: %w", err)
		}
		tier, err := sourceRequiredString(row.SubscriptionTier)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source tier: %w", err)
		}
		lastUsedAt, err := sourceNullTime(row.LastUsedAt)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source last used at: %w", err)
		}
		lastCapability, err := sourceNullString(row.LastCapability)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source last capability: %w", err)
		}
		businessStage, err := sourceNullString(row.BusinessStage)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source business stage: %w", err)
		}
		mainLineType, err := sourceNullString(row.MainLineType)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source main line type: %w", err)
		}
		userSegment, err := sourceNullString(row.UserSegment)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source user segment: %w", err)
		}
		painTag, err := sourceNullString(row.PainTag)
		if err != nil {
			return nil, fmt.Errorf("read hxc current source pain tag: %w", err)
		}
		monthlyChatQuota, err := sourceInt32(row.MonthlyChatQuota)
		if err != nil {
			return nil, err
		}
		currentPeriodUsed, err := sourceInt32(row.CurrentPeriodUsed)
		if err != nil {
			return nil, err
		}
		consultationLimit, err := sourceInt32(row.ConsultationLimit)
		if err != nil {
			return nil, err
		}
		consultationUsed, err := sourceInt32(row.ConsultationUsed)
		if err != nil {
			return nil, err
		}
		item := hxcport.SourceCurrent{
			HXCUserID:             row.HxcUserID,
			UnionID:               unionID.String,
			Phone:                 phone.String,
			SubscriptionTier:      tier,
			SubscriptionExpiresAt: nullTime(row.SubscriptionExpiresAt),
			MonthlyChatQuota:      monthlyChatQuota,
			CurrentPeriodUsed:     currentPeriodUsed,
			ConsultationLimit:     consultationLimit,
			ConsultationUsed:      consultationUsed,
			Sessions7D:            row.Sessions7d,
			Sessions30D:           row.Sessions30d,
			SessionsTotal:         row.SessionsTotal,
			UserMessages7D:        row.UserMessages7d,
			UserMessages30D:       row.UserMessages30d,
			UserMessagesTotal:     row.UserMessagesTotal,
			LastUsedAt:            nullTime(lastUsedAt),
			LastCapability:        nullString(lastCapability),
			BusinessStage:         nullString(businessStage),
			MainLineType:          nullString(mainLineType),
			UserSegment:           nullString(userSegment),
			PainTag:               nullString(painTag),
			SourceUpdatedAt:       row.SourceUpdatedAt.UTC(),
		}
		if json.Unmarshal(row.CapabilityUsage, &item.CapabilityUsage) != nil || json.Unmarshal(row.FocusTopics, &item.FocusTopics) != nil {
			return nil, errors.New("invalid hxc current source json")
		}
		if item.CapabilityUsage == nil {
			item.CapabilityUsage = map[string]hxcport.CapabilityUsage{}
		}
		if item.FocusTopics == nil {
			item.FocusTopics = []string{}
		}
		result = append(result, item)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit hxc source read: %w", err)
	}
	return result, nil
}

func sourceNullString(value any) (sql.NullString, error) {
	switch typed := value.(type) {
	case nil:
		return sql.NullString{}, nil
	case string:
		return sql.NullString{String: typed, Valid: true}, nil
	case []byte:
		return sql.NullString{String: string(typed), Valid: true}, nil
	default:
		return sql.NullString{}, fmt.Errorf("unexpected string type %T", value)
	}
}

func sourceRequiredString(value any) (string, error) {
	result, err := sourceNullString(value)
	if err != nil {
		return "", err
	}
	if !result.Valid {
		return "", errors.New("missing required string")
	}
	return result.String, nil
}

func sourceNullTime(value any) (sql.NullTime, error) {
	switch typed := value.(type) {
	case nil:
		return sql.NullTime{}, nil
	case time.Time:
		return sql.NullTime{Time: typed, Valid: true}, nil
	case string:
		return parseSourceTime(typed)
	case []byte:
		return parseSourceTime(string(typed))
	default:
		return sql.NullTime{}, fmt.Errorf("unexpected time type %T", value)
	}
}

func parseSourceTime(value string) (sql.NullTime, error) {
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", time.RFC3339Nano} {
		parsed, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return sql.NullTime{Time: parsed, Valid: true}, nil
		}
	}
	return sql.NullTime{}, fmt.Errorf("invalid time %q", value)
}

func sourceInt32(value int64) (int32, error) {
	if value < -1<<31 || value > 1<<31-1 {
		return 0, fmt.Errorf("hxc source integer %d exceeds int32", value)
	}
	return int32(value), nil
}

func nullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
