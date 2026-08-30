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
	rows, err := hxcsource.New(tx).ReadHXCCurrentView(ctx)
	if err != nil {
		return nil, fmt.Errorf("read hxc current view: %w", err)
	}
	result := make([]hxcport.SourceCurrent, 0, len(rows))
	for _, row := range rows {
		item := hxcport.SourceCurrent{
			HXCUserID:             row.HxcUserID,
			UnionID:               row.Unionid.String,
			Phone:                 row.Phone.String,
			SubscriptionTier:      row.SubscriptionTier,
			SubscriptionExpiresAt: nullTime(row.SubscriptionExpiresAt),
			MonthlyChatQuota:      row.MonthlyChatQuota,
			CurrentPeriodUsed:     row.CurrentPeriodUsed,
			ConsultationLimit:     row.ConsultationLimit,
			ConsultationUsed:      row.ConsultationUsed,
			Sessions7D:            row.Sessions7d,
			Sessions30D:           row.Sessions30d,
			SessionsTotal:         row.SessionsTotal,
			UserMessages7D:        row.UserMessages7d,
			UserMessages30D:       row.UserMessages30d,
			UserMessagesTotal:     row.UserMessagesTotal,
			LastUsedAt:            nullTime(row.LastUsedAt),
			LastCapability:        nullString(row.LastCapability),
			BusinessStage:         nullString(row.BusinessStage),
			MainLineType:          nullString(row.MainLineType),
			UserSegment:           nullString(row.UserSegment),
			PainTag:               nullString(row.PainTag),
			SourceUpdatedAt:       row.SourceUpdatedAt.UTC(),
		}
		if json.Unmarshal(row.CapabilityUsage, &item.CapabilityUsage) != nil || json.Unmarshal(row.FocusTopics, &item.FocusTopics) != nil {
			return nil, errors.New("invalid hxc current view json")
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
