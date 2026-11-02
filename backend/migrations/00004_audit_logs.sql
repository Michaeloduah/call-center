-- +goose Up
CREATE TABLE audit_logs (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    -- SET NULL keeps the trail after a tenant is deleted (entries become platform-only).
    tenant_id     uuid REFERENCES tenants (id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    actor_email   citext,
    action        text        NOT NULL CHECK (char_length(action) BETWEEN 1 AND 80),
    entity_type   text        NOT NULL CHECK (char_length(entity_type) BETWEEN 1 AND 40),
    entity_id     uuid,
    metadata      jsonb       NOT NULL DEFAULT '{}',
    ip            inet,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_tenant_created_idx ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at DESC);

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_logs
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

-- Append-only for the runtime role.
GRANT SELECT, INSERT ON audit_logs TO callcenter_app;

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
