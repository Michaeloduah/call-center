-- +goose Up
CREATE TABLE widgets (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    department_id   uuid        NOT NULL,
    name            text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    -- Public, non-secret identifier embedded in the tenant's website.
    public_key      text        NOT NULL UNIQUE CHECK (public_key ~ '^wk_[A-Za-z0-9_-]{22}$'),
    -- Sites allowed to embed the widget (enforced with CSP frame-ancestors).
    allowed_origins text[]      NOT NULL CHECK (cardinality(allowed_origins) BETWEEN 1 AND 20),
    is_active       boolean     NOT NULL DEFAULT true,
    button_label    text        NOT NULL DEFAULT 'Call us' CHECK (char_length(button_label) BETWEEN 1 AND 40),
    accent_color    text        NOT NULL DEFAULT '#4f46e5' CHECK (accent_color ~ '^#[0-9a-f]{6}$'),
    greeting        text        NOT NULL DEFAULT '' CHECK (char_length(greeting) <= 200),
    -- Caller form (D8): typed columns so the database rejects unknown modes.
    form_enabled    boolean     NOT NULL DEFAULT true,
    name_field      text        NOT NULL DEFAULT 'required' CHECK (name_field IN ('hidden', 'optional', 'required')),
    phone_field     text        NOT NULL DEFAULT 'optional' CHECK (phone_field IN ('hidden', 'optional', 'required')),
    email_field     text        NOT NULL DEFAULT 'hidden' CHECK (email_field IN ('hidden', 'optional', 'required')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT widgets_id_tenant_key UNIQUE (id, tenant_id),
    -- NO ACTION (checked at statement end): a department cannot be deleted while widgets route
    -- to it, yet deleting the whole tenant still cascades through both tables.
    FOREIGN KEY (department_id, tenant_id) REFERENCES departments (id, tenant_id) ON DELETE NO ACTION
);
CREATE INDEX widgets_tenant_created_idx ON widgets (tenant_id, created_at DESC);
CREATE INDEX widgets_department_idx ON widgets (department_id);
CREATE TRIGGER widgets_updated_at BEFORE UPDATE ON widgets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE calls (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id               uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    widget_id               uuid,
    department_id           uuid        NOT NULL,
    room_name               text        NOT NULL UNIQUE,
    status                  text        NOT NULL
        CHECK (status IN ('queued', 'ringing', 'active', 'completed', 'abandoned', 'missed', 'failed')),
    caller_name             text CHECK (char_length(caller_name) <= 120),
    caller_phone            text CHECK (char_length(caller_phone) <= 32),
    caller_email            citext CHECK (char_length(caller_email) <= 254),
    -- Snapshot of exactly what the caller agreed to (D6, D21).
    consent_text            text        NOT NULL,
    consent_given_at        timestamptz NOT NULL,
    caller_token_hash       bytea       NOT NULL UNIQUE,
    caller_token_expires_at timestamptz NOT NULL,
    caller_ip               inet,
    caller_user_agent       text CHECK (char_length(caller_user_agent) <= 512),
    queued_at               timestamptz NOT NULL DEFAULT now(),
    answered_at             timestamptz,
    ended_at                timestamptz,
    end_reason              text CHECK (char_length(end_reason) <= 40),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    -- Deleting a widget keeps its call history.
    FOREIGN KEY (widget_id, tenant_id) REFERENCES widgets (id, tenant_id) ON DELETE SET NULL (widget_id),
    -- Departments with call history are deactivated rather than deleted (see widgets FK note).
    FOREIGN KEY (department_id, tenant_id) REFERENCES departments (id, tenant_id) ON DELETE NO ACTION,
    CHECK ((status IN ('completed', 'abandoned', 'missed', 'failed')) = (ended_at IS NOT NULL))
);
CREATE INDEX calls_tenant_queued_idx ON calls (tenant_id, queued_at DESC);
CREATE INDEX calls_department_queue_idx ON calls (department_id, queued_at) WHERE status = 'queued';
CREATE INDEX calls_tenant_live_idx ON calls (tenant_id, status) WHERE status IN ('queued', 'ringing', 'active');
CREATE TRIGGER calls_updated_at BEFORE UPDATE ON calls
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE widgets ENABLE ROW LEVEL SECURITY;
ALTER TABLE widgets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON widgets
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE calls ENABLE ROW LEVEL SECURITY;
ALTER TABLE calls FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON calls
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON widgets TO callcenter_app;
-- Calls are history: the runtime role never deletes them (tenant deletion cascades as owner).
GRANT SELECT, INSERT, UPDATE ON calls TO callcenter_app;

-- +goose Down
DROP TABLE IF EXISTS calls;
DROP TABLE IF EXISTS widgets;
