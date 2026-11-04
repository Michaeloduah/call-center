-- +goose Up
-- Agent presence, call offers and call assignment (M3). See docs/M3-DESIGN.md.

CREATE TABLE agent_presence (
    user_id              uuid PRIMARY KEY,
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    -- The agent's own choice. "On a call" and "ringing" are derived from calls/offers, never stored.
    status               text        NOT NULL CHECK (status IN ('offline', 'available', 'away')),
    status_changed_at    timestamptz NOT NULL DEFAULT now(),
    -- Since when the agent has been free; the dispatcher offers calls to the longest-idle agent.
    idle_since           timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at    timestamptz NOT NULL DEFAULT now(),
    consecutive_timeouts smallint    NOT NULL DEFAULT 0 CHECK (consecutive_timeouts >= 0),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX agent_presence_routing_idx ON agent_presence (tenant_id, idle_since) WHERE status = 'available';
CREATE INDEX agent_presence_heartbeat_idx ON agent_presence (last_heartbeat_at) WHERE status <> 'offline';
CREATE TRIGGER agent_presence_updated_at BEFORE UPDATE ON agent_presence
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE calls ADD CONSTRAINT calls_id_tenant_key UNIQUE (id, tenant_id);
ALTER TABLE calls ADD COLUMN assigned_agent_id uuid;
-- Deleting a user keeps the call history.
ALTER TABLE calls ADD FOREIGN KEY (assigned_agent_id, tenant_id)
    REFERENCES users (id, tenant_id) ON DELETE SET NULL (assigned_agent_id);
-- An agent handles one call at a time (assumption 4 in the design), enforced by the database.
CREATE UNIQUE INDEX calls_one_active_per_agent_idx ON calls (assigned_agent_id) WHERE status = 'active';
CREATE INDEX calls_agent_idx ON calls (assigned_agent_id, queued_at DESC);

CREATE TABLE call_offers (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    call_id      uuid        NOT NULL,
    agent_id     uuid,
    offered_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    responded_at timestamptz,
    outcome      text        NOT NULL DEFAULT 'pending'
        CHECK (outcome IN ('pending', 'accepted', 'declined', 'timeout', 'cancelled')),
    CHECK ((outcome = 'pending') = (responded_at IS NULL)),
    FOREIGN KEY (call_id, tenant_id) REFERENCES calls (id, tenant_id) ON DELETE CASCADE,
    -- Deleting a user keeps the routing history.
    FOREIGN KEY (agent_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE SET NULL (agent_id)
);
-- A call has at most one live offer and an agent at most one live offer.
CREATE UNIQUE INDEX call_offers_one_pending_per_call_idx ON call_offers (call_id) WHERE outcome = 'pending';
CREATE UNIQUE INDEX call_offers_one_pending_per_agent_idx ON call_offers (agent_id) WHERE outcome = 'pending';
CREATE INDEX call_offers_expiry_idx ON call_offers (expires_at) WHERE outcome = 'pending';
CREATE INDEX call_offers_call_idx ON call_offers (call_id, offered_at);

ALTER TABLE agent_presence ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_presence FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agent_presence
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE call_offers ENABLE ROW LEVEL SECURITY;
ALTER TABLE call_offers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON call_offers
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

-- Offers are history: the runtime role never deletes them (tenant deletion cascades as owner).
GRANT SELECT, INSERT, UPDATE ON agent_presence, call_offers TO callcenter_app;

-- +goose Down
DROP TABLE IF EXISTS call_offers;
DROP INDEX IF EXISTS calls_agent_idx;
DROP INDEX IF EXISTS calls_one_active_per_agent_idx;
ALTER TABLE calls DROP COLUMN IF EXISTS assigned_agent_id;
ALTER TABLE calls DROP CONSTRAINT IF EXISTS calls_id_tenant_key;
DROP TABLE IF EXISTS agent_presence;
