# CallCenter

Multi-tenant customer-care calling over the internet. Businesses (tenants) connect their agents to customers through an embeddable web widget, with routing by department, recording and callbacks.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design, decisions and milestones.

## Status

**M1 — Foundation** is implemented:

- Auth: sign-in, self-service sign-up with email verification, password reset, invitations, revocable sessions, rate limiting.
- Platform admin: tenant CRUD (create with first-admin invite, edit, suspend/activate, delete), platform settings (default consent text), audit log.
- Tenant admin: user CRUD (invite, edit role, disable/enable, delete, resend invite), departments CRUD with agents, supervisors and weekly business hours, tenant settings.
- Tenant isolation enforced in repositories **and** by Postgres row-level security.

**M2 — Widgets & caller side** is implemented:

- Tenant admin: widget CRUD (department, allowed websites, button label/colour, greeting, caller-form fields) with an embed snippet.
- `widget.js` loader (Shadow DOM button + iframe) and the caller page: business-hours check, caller form, mandatory recording consent, queue position, hang-up.
- Public call API backed by self-hosted LiveKit: calls are created `queued`, the caller joins an audio-only room, and LiveKit webhooks (signature-verified) end calls when the caller leaves or the room closes.
- Embedding is restricted to each widget's allowed websites (CSP `frame-ancestors`, plus the loader's config endpoint refuses other origins).

Routing calls to agents starts in M3.

## Layout

```text
backend/   Go API (modular monolith) — cmd/api, cmd/cli, internal/<domain>, migrations/
web/       Next.js app (App Router, multi-page) — talks to the API from the server only
deploy/    docker-compose for local Postgres, Mailpit and LiveKit; sample widget test site
docs/      Architecture and decisions
```

## Local development

Requirements: Go 1.26.8+ (the LiveKit SDK requires it; `go` downloads it automatically), Node 22+, Docker.

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
- LiveKit (media server): ws://localhost:7880

Host ports are non-default (Postgres 55432, SMTP 11025) to avoid clashing with other local services.

### First platform admin

Platform admins are created from the CLI and receive an invitation email (visible in Mailpit):

```bash
make platform-admin EMAIL=you@example.com NAME="Your Name"
```

Businesses can also sign up themselves at `/signup`.

### Trying the widget

1. As a tenant admin, create a department (set its business hours) and a widget with `http://localhost:5500` in its allowed websites.
2. Serve the sample customer site:

```bash
make widget-test
```

3. Open `http://localhost:5500/?key=<the widget's public key>` and click the button. The browser must grant microphone access.

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
