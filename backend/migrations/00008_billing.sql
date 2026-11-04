-- +goose Up
-- Plans, subscriptions, usage, payments and webhook intake (M5). See docs/M5-DESIGN.md.
-- This migration writes tenant-scoped tables, so it runs in platform scope like the application does.
SELECT set_config('app.platform', 'on', true);

CREATE TABLE plans (
    id                         uuid PRIMARY KEY DEFAULT uuidv7(),
    code                       text        NOT NULL UNIQUE CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,39}$'),
    name                       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    description                text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    is_free                    boolean     NOT NULL DEFAULT false,
    is_public                  boolean     NOT NULL DEFAULT true,
    is_active                  boolean     NOT NULL DEFAULT true,
    -- NULL means unlimited.
    max_agents                 integer CHECK (max_agents >= 0),
    max_call_minutes_per_month integer CHECK (max_call_minutes_per_month >= 0),
    max_storage_bytes          bigint  CHECK (max_storage_bytes >= 0),
    max_departments            integer CHECK (max_departments >= 0),
    max_widgets                integer CHECK (max_widgets >= 0),
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT is_free OR is_active)
);
-- Exactly one free tier.
CREATE UNIQUE INDEX plans_one_free_idx ON plans (is_free) WHERE is_free;
CREATE TRIGGER plans_updated_at BEFORE UPDATE ON plans FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE plan_prices (
    id                       uuid PRIMARY KEY DEFAULT uuidv7(),
    plan_id                  uuid        NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    currency                 text        NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    interval                 text        NOT NULL CHECK (interval IN ('monthly', 'yearly')),
    amount_minor             bigint      NOT NULL CHECK (amount_minor > 0),
    overage_per_minute_minor bigint      NOT NULL DEFAULT 0 CHECK (overage_per_minute_minor >= 0),
    overage_per_gb_minor     bigint      NOT NULL DEFAULT 0 CHECK (overage_per_gb_minor >= 0),
    -- Derived from the currency (D19) and stored so the price is self-describing.
    provider                 text        NOT NULL CHECK (provider IN ('paystack', 'stripe', 'fake')),
    external_id              text,
    is_active                boolean     NOT NULL DEFAULT true,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (plan_id, currency, interval)
);
CREATE TRIGGER plan_prices_updated_at BEFORE UPDATE ON plan_prices FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE subscriptions (
    id                       uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id                uuid        NOT NULL UNIQUE REFERENCES tenants (id) ON DELETE CASCADE,
    plan_id                  uuid        NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    plan_price_id            uuid REFERENCES plan_prices (id) ON DELETE RESTRICT,
    provider                 text CHECK (provider IN ('paystack', 'stripe', 'fake')),
    external_customer_id     text,
    external_subscription_id text,
    -- Provider token for charging the saved card again (overage). Never returned by the API.
    external_authorization   text,
    -- The customer email the provider knows (needed to charge a saved Paystack authorization).
    billing_email            text,
    status                   text        NOT NULL CHECK (status IN ('active', 'past_due', 'cancelled', 'incomplete')),
    past_due_since           timestamptz,
    cancel_at_period_end     boolean     NOT NULL DEFAULT false,
    current_period_start     timestamptz NOT NULL,
    current_period_end       timestamptz NOT NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subscriptions_id_tenant_key UNIQUE (id, tenant_id),
    CHECK (current_period_end > current_period_start),
    CHECK ((status = 'past_due') = (past_due_since IS NOT NULL))
);
CREATE INDEX subscriptions_period_end_idx ON subscriptions (current_period_end) WHERE status IN ('active', 'past_due');
CREATE INDEX subscriptions_past_due_idx ON subscriptions (past_due_since) WHERE status = 'past_due';
CREATE UNIQUE INDEX subscriptions_external_idx ON subscriptions (provider, external_subscription_id) WHERE external_subscription_id IS NOT NULL;
CREATE TRIGGER subscriptions_updated_at BEFORE UPDATE ON subscriptions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payments (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id          uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subscription_id    uuid,
    kind               text        NOT NULL CHECK (kind IN ('subscription', 'overage')),
    provider           text        NOT NULL CHECK (provider IN ('paystack', 'stripe', 'fake')),
    external_reference text        NOT NULL,
    amount_minor       bigint      NOT NULL CHECK (amount_minor >= 0),
    currency           text        NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status             text        NOT NULL CHECK (status IN ('succeeded', 'failed', 'refunded')),
    failure            text CHECK (char_length(failure) <= 300),
    paid_at            timestamptz NOT NULL DEFAULT now(),
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, external_reference, status),
    FOREIGN KEY (subscription_id, tenant_id) REFERENCES subscriptions (id, tenant_id) ON DELETE SET NULL (subscription_id)
);
CREATE INDEX payments_tenant_idx ON payments (tenant_id, paid_at DESC);

CREATE TABLE usage_periods (
    id                   uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subscription_id      uuid,
    plan_id              uuid        NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    period_start         timestamptz NOT NULL,
    period_end           timestamptz NOT NULL,
    call_seconds         bigint      NOT NULL CHECK (call_seconds >= 0),
    storage_bytes        bigint      NOT NULL CHECK (storage_bytes >= 0),
    overage_minutes      integer     NOT NULL DEFAULT 0 CHECK (overage_minutes >= 0),
    overage_gb           integer     NOT NULL DEFAULT 0 CHECK (overage_gb >= 0),
    overage_amount_minor bigint      NOT NULL DEFAULT 0 CHECK (overage_amount_minor >= 0),
    currency             text,
    overage_payment_id   uuid REFERENCES payments (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    -- One snapshot per tenant and period: the claim that makes closing a period happen once.
    UNIQUE (tenant_id, period_start),
    FOREIGN KEY (subscription_id, tenant_id) REFERENCES subscriptions (id, tenant_id) ON DELETE SET NULL (subscription_id)
);

-- Each usage warning is emailed once per period.
CREATE TABLE usage_notices (
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    period_start timestamptz NOT NULL,
    cap          text        NOT NULL CHECK (cap IN ('minutes', 'storage')),
    threshold    smallint    NOT NULL CHECK (threshold IN (80, 100)),
    sent_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, period_start, cap, threshold)
);

-- Provider webhooks: stored for idempotency and debugging. Platform-only.
CREATE TABLE billing_webhook_events (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    provider          text        NOT NULL CHECK (provider IN ('paystack', 'stripe', 'fake')),
    external_event_id text        NOT NULL,
    type              text        NOT NULL,
    payload           jsonb       NOT NULL,
    processed_at      timestamptz,
    error             text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, external_event_id)
);

ALTER TABLE tenant_settings
    ADD COLUMN minutes_cap_policy text NOT NULL DEFAULT 'block' CHECK (minutes_cap_policy IN ('block', 'overage')),
    ADD COLUMN storage_cap_policy text NOT NULL DEFAULT 'block' CHECK (storage_cap_policy IN ('block', 'overage'));

CREATE INDEX calls_tenant_answered_idx ON calls (tenant_id, answered_at) WHERE answered_at IS NOT NULL;

-- Row-level security. Plans and prices are readable by everyone (tenants choose among them) and
-- writable only by the platform; the rest follow the usual tenant rule.
ALTER TABLE plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE plans FORCE ROW LEVEL SECURITY;
CREATE POLICY plans_read ON plans FOR SELECT USING (true);
CREATE POLICY plans_write ON plans FOR ALL USING (app_is_platform()) WITH CHECK (app_is_platform());

ALTER TABLE plan_prices ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan_prices FORCE ROW LEVEL SECURITY;
CREATE POLICY plan_prices_read ON plan_prices FOR SELECT USING (true);
CREATE POLICY plan_prices_write ON plan_prices FOR ALL USING (app_is_platform()) WITH CHECK (app_is_platform());

ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscriptions
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payments
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE usage_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON usage_periods
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE usage_notices ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON usage_notices
    USING (app_is_platform() OR tenant_id = app_tenant_id())
    WITH CHECK (app_is_platform() OR tenant_id = app_tenant_id());

ALTER TABLE billing_webhook_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_webhook_events FORCE ROW LEVEL SECURITY;
CREATE POLICY platform_only ON billing_webhook_events USING (app_is_platform()) WITH CHECK (app_is_platform());

-- Plans and prices are edited by the platform admin; billing records are history and never deleted.
GRANT SELECT, INSERT, UPDATE, DELETE ON plans, plan_prices TO callcenter_app;
GRANT SELECT, INSERT, UPDATE ON subscriptions, payments, usage_periods, billing_webhook_events TO callcenter_app;
GRANT SELECT, INSERT ON usage_notices TO callcenter_app;

-- The free tier every tenant starts on. Its numbers are a placeholder the platform admin can change.
INSERT INTO plans (code, name, description, is_free, max_agents, max_call_minutes_per_month, max_storage_bytes, max_departments, max_widgets)
VALUES ('free', 'Free', 'For trying the service out.', true, 2, 100, 524288000, 2, 1);

-- Existing tenants start on the free plan, in the current UTC calendar month.
INSERT INTO subscriptions (tenant_id, plan_id, status, current_period_start, current_period_end)
SELECT t.id, p.id, 'active',
       date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
       (date_trunc('month', now() AT TIME ZONE 'UTC') + interval '1 month') AT TIME ZONE 'UTC'
FROM tenants t CROSS JOIN plans p WHERE p.is_free;

-- +goose Down
DROP INDEX IF EXISTS calls_tenant_answered_idx;
ALTER TABLE tenant_settings DROP COLUMN IF EXISTS minutes_cap_policy, DROP COLUMN IF EXISTS storage_cap_policy;
DROP TABLE IF EXISTS billing_webhook_events;
DROP TABLE IF EXISTS usage_notices;
DROP TABLE IF EXISTS usage_periods;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS plan_prices;
DROP TABLE IF EXISTS plans;
