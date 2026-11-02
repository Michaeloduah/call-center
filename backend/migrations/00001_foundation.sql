-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

-- Runtime role used by the API. It must not own tables or have BYPASSRLS.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'callcenter_app') THEN
        CREATE ROLE callcenter_app NOLOGIN;
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO callcenter_app;

-- Row-level security context. The API sets these per transaction with set_config(..., true):
--   app.tenant_id  -> the tenant the request is scoped to
--   app.platform   -> 'on' only for platform-admin and system code paths
-- +goose StatementBegin
CREATE FUNCTION app_is_platform() RETURNS boolean
    LANGUAGE sql STABLE AS
$$ SELECT coalesce(current_setting('app.platform', true), '') = 'on' $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app_tenant_id() RETURNS uuid
    LANGUAGE sql STABLE AS
$$ SELECT nullif(current_setting('app.tenant_id', true), '')::uuid $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS set_updated_at();
DROP FUNCTION IF EXISTS app_tenant_id();
DROP FUNCTION IF EXISTS app_is_platform();
REVOKE USAGE ON SCHEMA public FROM callcenter_app;
