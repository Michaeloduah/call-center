# CallCenter

Multi-tenant customer-care calling over the internet. Businesses (tenants) connect their agents to customers through an embeddable web widget, with routing by department, recording and callbacks.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design, decisions and milestones.

## Status

**M1 — Foundation** is implemented:

- Auth: sign-in, self-service sign-up with email verification, password reset, invitations, revocable sessions, rate limiting.
- Platform admin: tenant CRUD (create with first-admin invite, edit, suspend/activate, delete), platform settings (default consent text), audit log.
- Tenant admin: user CRUD (invite, edit role, disable/enable, delete, resend invite), departments CRUD with agents, supervisors and weekly business hours, tenant settings.
- Tenant isolation enforced in repositories **and** by Postgres row-level security.

Calling (widget, LiveKit, routing, recording) starts in M2.

## Layout

```text
backend/   Go API (modular monolith) — cmd/api, cmd/cli, internal/<domain>, migrations/
web/       Next.js app (App Router, multi-page) — talks to the API from the server only
deploy/    docker-compose for local Postgres + Mailpit
docs/      Architecture and decisions
```

## Local development

Requirements: Go 1.26+, Node 22+, Docker.

```bash
make up
```

```bash
cp backend/.env.example backend/.env && cp web/.env.example web/.env.local
```

```bash
make migrate
```

```bash
make api
```

```bash
make web
```

- Web app: http://localhost:3100
- API: http://localhost:8080 (health: `/healthz`)
- Mailpit (all outgoing email in dev): http://localhost:18025

Host ports are non-default (Postgres 55432, SMTP 11025) to avoid clashing with other local services.

### First platform admin

Platform admins are created from the CLI and receive an invitation email (visible in Mailpit):

```bash
make platform-admin EMAIL=you@example.com NAME="Your Name"
```

Businesses can also sign up themselves at `/signup`.

## Tests

```bash
make test
```

Unit tests run anywhere; the row-level-security tests need `make up` (they migrate the `callcenter_test` database).

```bash
cd web && npm run typecheck && npm run lint && npm run build
```

## Database roles

- `callcenter` owns the schema and runs migrations (`MIGRATIONS_DATABASE_URL`).
- `callcenter_app` is the runtime role (`DATABASE_URL`). It does not own tables and has no `BYPASSRLS`, so row-level security always applies to the API. In production, create this role with a password before running migrations.
