-- +goose Up
-- Recordings, in-call chat and callback requests (M4). See docs/M4-DESIGN.md.

CREATE TABLE recordings (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    call_id          uuid        NOT NULL,
    -- Known once the recorder has accepted the request; webhooks find the row through it.
    egress_id        text UNIQUE,
    status           text        NOT NULL CHECK (status IN ('starting', 'recording', 'ready', 'failed')),
    storage_key      text,
    content_type     text,
    size_bytes       bigint CHECK (size_bytes >= 0),
    duration_seconds integer CHECK (duration_seconds >= 0),
    started_at       timestamptz NOT NULL DEFAULT now(),
    ended_at         timestamptz,
    error            text CHECK (char_length(error) <= 500),
    -- Soft delete (D7): hidden from the tenant, file kept, still counted for storage (D17).
    deleted_at       timestamptz,
    deleted_by       uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT recordings_call_key UNIQUE (call_id),
    CONSTRAINT recordings_id_tenant_key UNIQUE (id, tenant_id),
    CHECK (status <> 'ready' OR (storage_key IS NOT NULL AND size_bytes IS NOT NULL)),
    CHECK ((deleted_at IS NULL) = (deleted_by IS NULL) OR deleted_at IS NOT NULL),
    FOREIGN KEY (call_id, tenant_id) REFERENCES calls (id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (deleted_by, tenant_id) REFERENCES users (id, tenant_id) ON DELETE SET NULL (deleted_by)
);
CREATE INDEX recordings_tenant_created_idx ON recordings (tenant_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX recordings_tenant_size_idx ON recordings (tenant_id) INCLUDE (size_bytes);
CREATE INDEX recordings_deleted_idx ON recordings (deleted_at DESC) WHERE deleted_at IS NOT NULL;
CREATE INDEX recordings_unfinished_idx ON recordings (started_at) WHERE status IN ('starting', 'recording');
CREATE TRIGGER recordings_updated_at BEFORE UPDATE ON recordings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE call_messages (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id      uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    call_id        uuid        NOT NULL,
    sender_type    text        NOT NULL CHECK (sender_type IN ('caller', 'agent')),
    sender_user_id uuid,
    sender_name    text        NOT NULL CHECK (char_length(sender_name) BETWEEN 1 AND 120),
    body           text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    sent_at        timestamptz NOT NULL DEFAULT now(),
    CHECK ((sender_type = 'agent') OR sender_user_id IS NULL),
    FOREIGN KEY (call_id, tenant_id) REFERENCES calls (id, tenant_id) ON DELETE CASCADE,
    -- Deleting a user keeps the transcript.
    FOREIGN KEY (sender_user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE SET NULL (sender_user_id)
);
CREATE INDEX call_messages_call_idx ON call_messages (call_id, sent_at, id);

CREATE TABLE callback_requests (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    department_id    uuid        NOT NULL,
    call_id          uuid,
    name             text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    phone            text CHECK (char_length(phone) <= 32),
    email            citext CHECK (char_length(email) <= 254),
    reason           text        NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    status           text        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'claimed', 'completed', 'cancelled')),
    claimed_by       uuid,
    claimed_at       timestamptz,
    resolution_notes text        NOT NULL DEFAULT '' CHECK (char_length(resolution_notes) <= 2000),
    resolved_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    -- Without a way to reach the caller there is nothing to call back.
    CHECK (phone IS NOT NULL OR email IS NOT NULL),
    CHECK ((status IN ('completed', 'cancelled')) = (resolved_at IS NOT NULL)),
    CONSTRAINT callback_requests_id_tenant_key UNIQUE (id, tenant_id),
    -- Departments with callback history are deactivated rather than deleted (as with calls).
    FOREIGN KEY (department_id, tenant_id) REFERENCES departments (id, tenant_id) ON DELETE NO ACTION,
    FOREIGN KEY (call_id, tenant_id) REFERENCES calls (id, tenant_id) ON DELETE SET NULL (call_id),
    FOREIGN KEY (claimed_by, tenant_id) REFERENCES users (id, tenant_id) ON DELETE SET NULL (claimed_by)
);
CREATE INDEX callback_requests_department_idx ON callback_requests (department_id, status, created_at);
CREATE INDEX callback_requests_tenant_idx ON callback_requests (tenant_id, created_at DESC);
-- A missed call can produce at most one callback request.
CREATE UNIQUE INDEX callback_requests_call_idx ON callback_requests (call_id) WHERE call_id IS NOT NULL;
CREATE INDEX callback_requests_claimed_idx ON callback_requests (claimed_by) WHERE status = 'claimed';
CREATE TRIGGER callback_requests_updated_at BEFORE UPDATE ON callback_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE recordings ENABLE ROW LEVEL SECURITY;
ALTER TABLE recordings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recordings
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE call_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE call_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON call_messages
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE callback_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE callback_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON callback_requests
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

-- History is kept: the runtime role never deletes these rows (tenant deletion cascades as owner).
GRANT SELECT, INSERT, UPDATE ON recordings, call_messages, callback_requests TO callcenter_app;

-- +goose Down
DROP TABLE IF EXISTS callback_requests;
DROP TABLE IF EXISTS call_messages;
DROP TABLE IF EXISTS recordings;
