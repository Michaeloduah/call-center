package presence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
)

type Repository interface {
	// Departments lists the departments the user is an agent in. An empty list means they cannot take calls.
	Departments(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	// SetStatus stores the agent's choice and counts them as alive right now. Becoming available
	// makes the agent idle from this instant and clears the timeout streak.
	SetStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status) error
	// Heartbeat marks the agent alive. It returns their stored status (offline if they have none).
	Heartbeat(ctx context.Context, tenantID, userID uuid.UUID) (Status, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func (pgRepository) Departments(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT department_id FROM department_members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent departments: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan agent department: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (pgRepository) SetStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	// idle_since moves only when the agent becomes available, so toggling away/available is the
	// way to go to the back of the line; a repeated "available" keeps their place.
	if _, err := q.Exec(ctx, `
		INSERT INTO agent_presence (user_id, tenant_id, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
		    idle_since = CASE WHEN $3 = 'available' AND agent_presence.status <> 'available' THEN now() ELSE agent_presence.idle_since END,
		    status_changed_at = CASE WHEN agent_presence.status <> $3 THEN now() ELSE agent_presence.status_changed_at END,
		    consecutive_timeouts = CASE WHEN $3 = 'available' THEN 0 ELSE agent_presence.consecutive_timeouts END,
		    status = $3,
		    last_heartbeat_at = now()`,
		userID, tenantID, string(status)); err != nil {
		return fmt.Errorf("set presence: %w", err)
	}
	return nil
}

func (pgRepository) Heartbeat(ctx context.Context, tenantID, userID uuid.UUID) (Status, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return "", err
	}
	var status Status
	if err := q.QueryRow(ctx, `
		INSERT INTO agent_presence (user_id, tenant_id, status) VALUES ($1, $2, 'offline')
		ON CONFLICT (user_id) DO UPDATE SET last_heartbeat_at = now()
		RETURNING status`, userID, tenantID).Scan(&status); err != nil {
		return "", fmt.Errorf("heartbeat: %w", err)
	}
	return status, nil
}
