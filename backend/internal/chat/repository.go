package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
)

type Repository interface {
	// InsertFromCaller saves a caller's message if the call is active. It returns the saved
	// message and the call's media room, or nil when the call is not active.
	InsertFromCaller(ctx context.Context, callID uuid.UUID, body string) (*Message, string, error)
	// InsertFromAgent does the same for the call's assigned agent.
	InsertFromAgent(ctx context.Context, callID, agentID uuid.UUID, agentName, body string) (*Message, string, error)
	List(ctx context.Context, callID uuid.UUID) ([]Message, error)
	// IsAssignedAgent reports whether the call was handled by agentID.
	IsAssignedAgent(ctx context.Context, callID, agentID uuid.UUID) (bool, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func insert(ctx context.Context, sql string, args ...any) (*Message, string, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, "", err
	}
	var m Message
	var room string
	err = q.QueryRow(ctx, sql, args...).Scan(&m.ID, &m.Sender, &m.Name, &m.Body, &m.SentAt, &room)
	if database.IsNoRows(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("insert chat message: %w", err)
	}
	return &m, room, nil
}

func (pgRepository) InsertFromCaller(ctx context.Context, callID uuid.UUID, body string) (*Message, string, error) {
	return insert(ctx, `
		WITH m AS (
		    INSERT INTO call_messages (tenant_id, call_id, sender_type, sender_name, body)
		    SELECT c.tenant_id, c.id, 'caller', coalesce(c.caller_name, 'Caller'), $2
		    FROM calls c WHERE c.id = $1 AND c.status = 'active'
		    RETURNING id, sender_type, sender_name, body, sent_at, call_id
		)
		SELECT m.id, m.sender_type, m.sender_name, m.body, m.sent_at, c.room_name FROM m JOIN calls c ON c.id = m.call_id`,
		callID, body)
}

func (pgRepository) InsertFromAgent(ctx context.Context, callID, agentID uuid.UUID, agentName, body string) (*Message, string, error) {
	return insert(ctx, `
		WITH m AS (
		    INSERT INTO call_messages (tenant_id, call_id, sender_type, sender_user_id, sender_name, body)
		    SELECT c.tenant_id, c.id, 'agent', $2, $3, $4
		    FROM calls c WHERE c.id = $1 AND c.status = 'active' AND c.assigned_agent_id = $2
		    RETURNING id, sender_type, sender_name, body, sent_at, call_id
		)
		SELECT m.id, m.sender_type, m.sender_name, m.body, m.sent_at, c.room_name FROM m JOIN calls c ON c.id = m.call_id`,
		callID, agentID, agentName, body)
}

func (pgRepository) List(ctx context.Context, callID uuid.UUID) ([]Message, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT id, sender_type, sender_name, body, sent_at FROM call_messages
		WHERE call_id = $1 ORDER BY sent_at, id`, callID)
	if err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Sender, &m.Name, &m.Body, &m.SentAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (pgRepository) IsAssignedAgent(ctx context.Context, callID, agentID uuid.UUID) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM calls WHERE id = $1 AND assigned_agent_id = $2)`, callID, agentID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check assigned agent: %w", err)
	}
	return ok, nil
}
