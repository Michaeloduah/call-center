-- +goose Up
CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    slug       text        NOT NULL UNIQUE
        CHECK (char_length(slug) BETWEEN 3 AND 60 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    status     text        NOT NULL CHECK (status IN ('pending_verification', 'active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tenants_created_at_idx ON tenants (created_at DESC);
CREATE TRIGGER tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Typed columns (not jsonb) so the database enforces valid ranges.
CREATE TABLE tenant_settings (
    tenant_id               uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    offer_timeout_seconds   integer     NOT NULL DEFAULT 20 CHECK (offer_timeout_seconds BETWEEN 5 AND 120),
    max_queue_wait_seconds  integer     NOT NULL DEFAULT 300 CHECK (max_queue_wait_seconds BETWEEN 30 AND 3600),
    consent_text_override   text        CHECK (char_length(consent_text_override) BETWEEN 1 AND 2000),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER tenant_settings_updated_at BEFORE UPDATE ON tenant_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id         uuid REFERENCES tenants (id) ON DELETE CASCADE,
    email             citext      NOT NULL UNIQUE CHECK (char_length(email) <= 254),
    password_hash     text,
    full_name         text        NOT NULL CHECK (char_length(full_name) BETWEEN 1 AND 120),
    role              text        NOT NULL CHECK (role IN ('platform_admin', 'tenant_admin', 'supervisor', 'agent')),
    status            text        NOT NULL CHECK (status IN ('invited', 'active', 'disabled')),
    email_verified_at timestamptz,
    last_login_at     timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_platform_admin_has_no_tenant CHECK ((role = 'platform_admin') = (tenant_id IS NULL)),
    CONSTRAINT users_active_has_password CHECK (status <> 'active' OR password_hash IS NOT NULL),
    -- Target for composite foreign keys that guarantee same-tenant references.
    CONSTRAINT users_id_tenant_key UNIQUE (id, tenant_id)
);
CREATE INDEX users_tenant_created_idx ON users (tenant_id, created_at DESC);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Sessions and tokens are reached only through a hashed secret, so they carry no tenant_id
-- and are accessed exclusively through the platform-scoped auth code path.
CREATE TABLE sessions (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea       NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    ip         inet,
    user_agent text CHECK (char_length(user_agent) <= 512),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE user_tokens (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text        NOT NULL CHECK (purpose IN ('verify_email', 'reset_password', 'invite')),
    token_hash bytea       NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_tokens_user_purpose_idx ON user_tokens (user_id, purpose) WHERE used_at IS NULL;

CREATE TABLE platform_settings (
    id                   smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    default_consent_text text        NOT NULL CHECK (char_length(default_consent_text) BETWEEN 1 AND 2000),
    updated_by           uuid REFERENCES users (id) ON DELETE SET NULL,
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER platform_settings_updated_at BEFORE UPDATE ON platform_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
INSERT INTO platform_settings (default_consent_text)
VALUES ('This call will be recorded for quality and training purposes. By continuing, you consent to the recording.');

-- Row-level security
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenants
    USING (app_is_platform() OR id = app_tenant_id())
    WITH CHECK (app_is_platform() OR id = app_tenant_id());

ALTER TABLE tenant_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_settings
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

-- Platform admins (tenant_id IS NULL) are only visible in platform scope.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, tenant_settings, users, sessions, user_tokens TO callcenter_app;
GRANT SELECT, UPDATE ON platform_settings TO callcenter_app;

-- +goose Down
DROP TABLE IF EXISTS platform_settings;
DROP TABLE IF EXISTS user_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenant_settings;
DROP TABLE IF EXISTS tenants;
