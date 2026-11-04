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

**M3 — Routing & consoles** is implemented:

- Agents set themselves available / away / offline from the agent console (`/agent`). The console keeps a heartbeat; an agent whose browser stops responding goes offline automatically.
- Waiting calls are offered to the longest-idle available agent of the widget's department (oldest call first, an agent can serve several departments). Offers time out after the tenant's offer timeout and move to the next agent; two unanswered offers in a row set an agent away; calls nobody answers within the maximum wait become `missed`.
- Accepting joins the agent to the call's media room. Either side can end the call; if an agent's connection drops the call ends and they go offline.
- Supervisors and tenant admins get a live board (`/live`) of queues, calls in progress and agent states, limited to supervised departments. Updates reach browsers through Server-Sent Events backed by Postgres LISTEN/NOTIFY (no Redis needed yet).
- The widget shows "an agent is being notified", "nobody is available" and "nobody answered" states.

**M4 — Chat, recording, callbacks & history** is implemented:

- Every answered call is recorded (LiveKit Egress → MinIO). Admins, and supervisors for their departments, play or download a recording through a 5-minute presigned link; each issue is audit-logged. Admins can soft-delete a recording (hidden, file kept); the platform admin sees deleted recordings and can restore them. Agents cannot play recordings.
- In-call text chat between caller and agent (saved, delivered through LiveKit, only while the call is active) and a full transcript on the call detail page.
- Callback requests when the department is closed, nobody is online, or a call goes unanswered; agents claim and complete them, admins can assign or cancel them and are emailed about new ones.
- Call history (`/calls`) and callbacks (`/callbacks`) for every tenant role, scoped by what each role may see; the platform admin has `/platform/recordings/deleted`.

**M5 — Plans, billing & limits** is implemented:

- Platform admins manage plans and prices (`/platform/plans`); every tenant starts on the free plan and upgrades from `/admin/billing` through a hosted checkout. A plan only changes when a verified provider webhook arrives.
- Plan limits (agent seats, departments, widgets, call minutes per month, recording storage) are enforced in the services. When a minutes or storage cap blocks new calls, callers get the callback form. Tenant admins choose per cap whether to block or pay for the extra (paid plans only); overage is billed once when the period closes.
- A failed payment starts a 14-day grace period, then the tenant falls back to the free plan (nothing is deleted). Admins are emailed at 80% and 100% of each cap and on payment failure.
- **Payments are simulated in development** (`BILLING_FAKE`, on by default outside production). Paystack and Stripe adapters exist and are tested against local fake servers and signed samples, but **have not been run against the real providers**; the checklist is in `docs/M5-DESIGN.md` §17.

Known limit: the recorder takes 5–8 seconds to start, so the first seconds of a call are not captured (see `docs/M4-DESIGN.md` §17).

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
- MinIO (recordings): API http://localhost:19000, console http://localhost:19001 (login in `deploy/docker-compose.yml`)

Host ports are non-default (Postgres 55432, SMTP 11025, MinIO 19000) to avoid clashing with other local services. The stack also runs Redis and LiveKit Egress for recording; Egress starts a headless browser per recorded call, so a busy machine can make recordings fail with "Start signal not received". The API creates the `recordings` bucket on startup. After changing `deploy/livekit/*.yaml`, restart `livekit` and `egress` (`docker compose -f deploy/docker-compose.yml restart livekit egress`).

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
