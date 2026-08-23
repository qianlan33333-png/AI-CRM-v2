package automationfixture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidAgentConfiguration = errors.New("invalid automation agent configuration fixture")

// CreateAgentConfiguration creates an active Automation-owned agent in the
// transaction that owns the surrounding acceptance scenario.
func CreateAgentConfiguration(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	if tx == nil || code == "" {
		return 0, ErrInvalidAgentConfiguration
	}
	var id int64
	if err := tx.QueryRow(ctx, `
INSERT INTO automation_agent_configurations
  (agent_name, agent_code, automation_type, status, created_by, updated_by, created_at, updated_at)
VALUES ('Audience local configuration agent', $1::text, 'agent', 'active', 7, 7, now(), now())
RETURNING id`, code).Scan(&id); err != nil {
		return 0, fmt.Errorf("create automation-owned acceptance agent: %w", err)
	}
	return id, nil
}
