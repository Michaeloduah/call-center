-- +goose Up
CREATE TABLE departments (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    timezone    text        NOT NULL DEFAULT 'UTC' CHECK (char_length(timezone) BETWEEN 1 AND 64),
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT departments_id_tenant_key UNIQUE (id, tenant_id)
);
CREATE UNIQUE INDEX departments_tenant_name_key ON departments (tenant_id, lower(name));
CREATE TRIGGER departments_updated_at BEFORE UPDATE ON departments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Composite foreign keys (x_id, tenant_id) guarantee every link stays inside one tenant.
CREATE TABLE department_business_hours (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id     uuid     NOT NULL,
    department_id uuid     NOT NULL,
    weekday       smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0 = Sunday
    opens_at      time     NOT NULL,
    closes_at     time     NOT NULL,
    CHECK (opens_at < closes_at),
    FOREIGN KEY (department_id, tenant_id) REFERENCES departments (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX department_business_hours_department_idx ON department_business_hours (department_id, weekday, opens_at);

CREATE TABLE department_members (
    department_id uuid        NOT NULL,
    user_id       uuid        NOT NULL,
    tenant_id     uuid        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (department_id, user_id),
    FOREIGN KEY (department_id, tenant_id) REFERENCES departments (id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX department_members_user_idx ON department_members (user_id);

CREATE TABLE department_supervisors (
    department_id uuid        NOT NULL,
    user_id       uuid        NOT NULL,
    tenant_id     uuid        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (department_id, user_id),
    FOREIGN KEY (department_id, tenant_id) REFERENCES departments (id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX department_supervisors_user_idx ON department_supervisors (user_id);

ALTER TABLE departments ENABLE ROW LEVEL SECURITY;
ALTER TABLE departments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON departments
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE department_business_hours ENABLE ROW LEVEL SECURITY;
ALTER TABLE department_business_hours FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON department_business_hours
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE department_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE department_members FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON department_members
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE department_supervisors ENABLE ROW LEVEL SECURITY;
ALTER TABLE department_supervisors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON department_supervisors
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE
    ON departments, department_business_hours, department_members, department_supervisors
    TO callcenter_app;

-- +goose Down
DROP TABLE IF EXISTS department_supervisors;
DROP TABLE IF EXISTS department_members;
DROP TABLE IF EXISTS department_business_hours;
DROP TABLE IF EXISTS departments;
