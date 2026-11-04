# M4 — Chat, Recording, Callbacks & History (design, v0.2)

Status: **implemented** (see [Implementation notes](#17-implementation-notes)). Builds on `ARCHITECTURE.md` v0.5 (§5.2, §6, §7, §9) and the M3 code.
Labels: **Decision** (already confirmed), **Proposal** (my recommendation), **Assumption**, **Open question**.

**Goal (from §14):** an admin plays a recording and reads its transcript. A missed caller leaves a callback that an agent claims and completes.

**Not in M4:** plans, quotas and billing (M5), retention/automatic deletion, outbound calling.
**M4 closes the compliance gap of M3:** after M4 every answered call is recorded (D6). Until then no real callers.

---

## 1. Scope

| Area | Included |
|------|----------|
| Recording | LiveKit Egress → MinIO (S3), recording metadata, playback/download by presigned URL (audit-logged), tenant soft delete, platform restore |
| Chat | Text chat during an `active` call (D14, D24), persisted, delivered through LiveKit |
| Callbacks | Public callback form (after `missed`, outside hours, nobody online), agent claim/complete workflow, admin oversight, notifications |
| History | Call list and call detail (caller, consent snapshot, offer timeline, recording, transcript), scoped per role |
| Widget | Chat panel, "this call is recorded" notice, callback form in the closed / no-agents / missed states |
| Agent console | Chat panel, callbacks waiting indicator |

---

## 2. Recording

### 2.1 Infrastructure (the riskiest part: spike first)

New dev services in `deploy/docker-compose.yml`: **redis**, **minio** (+ a one-shot bucket creation), **egress** (`livekit/egress`).

- **Redis is required by LiveKit Egress** (the server and egress talk through it). It is used by LiveKit only; the Go API still does not touch Redis (D26 stands: "until LiveKit needs it"). `livekit.yaml` gains a `redis:` block.
- **Egress must reach the SFU's media port.** LiveKit advertises `node_ip: 127.0.0.1` in dev, which inside another container means that container itself. Proposal: run egress with `network_mode: "service:livekit"` so both share one network namespace (`127.0.0.1` is then the SFU), with `ws_url: ws://localhost:7880`. Egress's headless Chrome also needs `shm_size` and extra capabilities. **This is unverified on macOS Docker Desktop**, so implementation step 1 is a spike that records one call end to end before anything is built around it. Fallback if it fails: egress with `network: host`, or a dev-only `node_ip` on the compose network.
- Production: egress is CPU-heavy and runs on its own node(s) (§12), with a real `node_ip`; MinIO is replaced by any S3-compatible store (D15).

### 2.2 Choice of egress type
**Proposal:** `RoomCompositeEgress` with `audio_only`, OGG/Opus, one **mixed** file per call (matches v0.4 and gives admins one file to play). It runs a headless browser, so it costs roughly one CPU core per concurrent call. Alternative: per-participant `TrackEgress` (no browser, cheap) plus an ffmpeg mix step afterwards: cheaper to run, more code and a second failure mode. I recommend composite for M4 and revisiting only if egress cost matters.
Expected size is roughly 0.5 MB per minute (to be measured in the spike; M5's storage quota needs the real figure).

### 2.3 Lifecycle
```text
accept (routing.Accept commits)
  └─ after commit, in the background (never delays the agent):
       recording row: status = 'starting'                  (own transaction)
       media.StartRecording(room, s3 target) → egress id    (LiveKit call)
       row: status = 'recording', egress_id                 (on failure: 'failed' + error)
call ends → room deleted → egress finalises the file
LiveKit webhook egress_ended (signature-verified)
  └─ row: 'ready' (size, duration, ended_at)  or 'failed' (error)    -- conditional update, idempotent
sweeper (existing 2 s loop)
  └─ 'starting' older than 1 min, or 'recording' more than 10 min after the call ended → 'failed'
     (no completion event arrived)
```
- Recording starts at **accept**, not when the agent's audio connects (v0.4 §5 said "participant_joined"): egress needs a few seconds to start, and starting at accept gives it a head start so the opening words are less likely to be lost. Queue/ringing time is never recorded.
- **A recording failure never ends the call** (v0.4 §5.3). The call detail shows "Recording unavailable" with the reason; the agent console shows a small "this call is not being recorded" warning when the row is `failed`, so it is visible live.
- Object key: `tenants/{tenantId}/recordings/{callId}.ogg` in a private bucket. The key is never returned to clients.

### 2.4 Access, soft delete, restore (D7, D17)
| Action | Who | Effect |
|--------|-----|--------|
| Play / download | tenant admin; supervisor for their departments (v0.4 §13.1) | `POST /v1/calls/{id}/recording/access {disposition}` → presigned GET URL (5 min), **audit-logged** (`recording.played` / `recording.downloaded`) |
| Soft delete | tenant admin | `DELETE /v1/calls/{id}/recording` → `deleted_at`, `deleted_by`, audit `recording.deleted`. Hidden from every tenant query, file kept, still counted for storage (D17) |
| List deleted / play / restore | platform admin | `GET /v1/platform/recordings/deleted`, `POST …/{id}/access`, `POST …/{id}/restore` (clears the delete, audit `recording.restored`) |

- A GET never writes an audit row; playback goes through `POST …/access` so the audit entry is a deliberate side effect of a deliberate request.
- Browsers fetch the file directly from storage with the presigned URL (`<audio src>`; range requests work; no CORS needed for media elements). Streaming large files through the API/Next.js would add cost and complexity for nothing.
- Presigning must use the host the **browser** uses, so storage has three endpoints in config: API→store (`STORAGE_ENDPOINT`), egress→store (`STORAGE_EGRESS_ENDPOINT`, e.g. `http://minio:9000`) and browser→store (`STORAGE_PUBLIC_ENDPOINT`).
- Presigned URLs are bearer links valid for 5 minutes: short TTL, never logged, `Cache-Control: no-store` on the access response.

### 2.5 Provider interface changes
`media.Provider` gains `StartRecording(ctx, room, Target) (egressID, error)` and `StopRecording`. `ParseWebhook` also decodes egress events (`egress_ended`: egress id, status, size, duration, error). A new `storage.Store` interface (`Presign`, `EnsureBucket`, `EgressTarget`) wraps `minio-go/v7` (v0.4's planned dependency; presigned SigV4 URLs are not worth hand-rolling).

---

## 3. Chat

Exactly the v0.4 §5.2 design, now with details:

- `POST /v1/public/calls/{id}/messages` (caller token) and `POST /v1/agent/calls/{id}/messages` (agent session). The API **validates, persists** in `call_messages`, then pushes the message into the room with LiveKit `SendData` (reliable, topic `chat`). One write path: the saved transcript and what users saw cannot diverge, and clients cannot inject unsaved messages (tokens already forbid data publishing).
- Accepted **only while the call is `active`** (D24), from the caller of that call or the assigned agent.
- Plain text, 1–1000 characters, trimmed, rendered escaped (React text nodes, never HTML). Rate limit: 20 messages per minute per call.
- Payload to clients: `{id, sender: "caller"|"agent", name, body, sent_at}`; clients de-duplicate by `id`.
- History for reloads: `GET /v1/public/calls/{id}/messages` and `GET /v1/agent/calls/{id}/messages`; admin/supervisor see the transcript in the call detail.
- Supervisors cannot listen in or chat (D9); chat is private to the two participants until it is read from history.

## 4. Callbacks (D5, D23)

- **Created from the widget** in three situations: the call went `missed`, the department is **closed**, or **nobody is online**. All three show the same form (name, phone, email, reason). Linked to the call when there is one.
- **Public endpoints:** `POST /v1/public/widgets/{key}/callbacks` (no call) and `POST /v1/public/calls/{id}/callback` (after a `missed` call, caller token; prefilled from the call). Rate limit: 5 per 10 minutes per IP.
- **Validation:** name required (120 chars); **at least one of phone or email** is required whatever the caller-form config says (without it nobody can call back); phone/email formats as in M2; reason optional (500 chars). A short privacy note is shown instead of the recording consent (no audio is captured).
- **Workflow** (status `open → claimed → completed | cancelled`):

| Action | Who | Rule |
|--------|-----|------|
| claim | member of the callback's department | `open → claimed`, conditional update (two agents cannot both win) |
| release | the claimer, or a tenant admin | `claimed → open` |
| complete (+ notes ≤ 2000) | the claimer, or a tenant admin | `claimed → completed` |
| cancel | tenant admin | `open/claimed → cancelled` |
| assign | tenant admin | sets the claimer to a department member (v0.4 "reassign") |

- Supervisors have no callback actions (D9: live view only).
- **Notifications (v0.4 §6):** new callback → realtime hint to every department member's console (the agent console shows "N callbacks waiting") and an **email to tenant admins** through the existing `Mailer`. **Assumption:** one email per callback is acceptable in v1; a per-tenant digest or opt-out is a later setting.
- **Assumption (v0.4):** the callback is returned outside the platform (phone/email). Outbound calling is not part of the product.

## 5. Call history

One read model (`history` package) behind one set of endpoints; the **scope is applied in the service, not the UI**:

| Role | Sees |
|------|------|
| tenant admin | all calls of the tenant |
| supervisor | calls of their departments (D22) |
| agent | calls they handled (`assigned_agent_id = me`) |

- `GET /v1/calls` — filters: status, department, agent, date range, text (caller name/phone/email), pagination.
- `GET /v1/calls/{id}` — caller details and the **consent snapshot** (text + time), timeline (queued, each offer with agent/outcome, answered, ended, reason), recording metadata (status, duration, size; never the key), transcript.
- Caller IP and user agent are stored for abuse handling but **not shown** in the UI.
- Recording actions (play, delete) appear only for roles allowed to use them (§2.4). Agents see history and transcript but **not recordings** (open question 2).

## 6. Data model (migration `00007_recordings_chat_callbacks.sql`)

```text
recordings
  id, tenant_id, call_id UNIQUE (composite FK → calls ON DELETE CASCADE), egress_id UNIQUE NULL,
  status ('starting'|'recording'|'ready'|'failed'), storage_key, content_type,
  size_bytes, duration_seconds, started_at, ended_at, error,
  deleted_at NULL, deleted_by (FK users, SET NULL)
  CHECK (status <> 'ready' OR (storage_key IS NOT NULL AND size_bytes IS NOT NULL))
  INDEX (tenant_id, created_at DESC) WHERE deleted_at IS NULL     -- tenant listing
  INDEX (tenant_id) INCLUDE (size_bytes)                           -- storage quota, includes deleted (D17, M5)
  INDEX (deleted_at) WHERE deleted_at IS NOT NULL                  -- platform "deleted" list

call_messages
  id, tenant_id, call_id (composite FK CASCADE), sender_type ('caller'|'agent'),
  sender_user_id NULL (SET NULL), sender_name, body text CHECK (char_length(body) BETWEEN 1 AND 1000), sent_at
  INDEX (call_id, sent_at, id)

callback_requests
  id, tenant_id, department_id (composite FK, NO ACTION like calls), call_id NULL (composite FK, SET NULL),
  name, phone, email, reason, status ('open'|'claimed'|'completed'|'cancelled'),
  claimed_by NULL (SET NULL), claimed_at, resolution_notes, resolved_at,
  CHECK (phone IS NOT NULL OR email IS NOT NULL)
  CHECK status/claimed_by/resolved_at consistency
  INDEX (department_id, status, created_at)
```
- RLS `tenant_isolation` and composite tenant FKs as everywhere. The runtime role gets SELECT/INSERT/UPDATE (no DELETE; history is kept, tenant deletion cascades as owner).
- `recordings` keeps the `call_id UNIQUE`: one recording per call.
- The platform-admin queries run in platform scope, the only code path allowed to see soft-deleted rows (tenant queries filter `deleted_at IS NULL` in SQL, and the list index is partial on it).

## 7. API summary

| Method & path | Who |
|---------------|-----|
| `POST /v1/public/calls/{id}/messages`, `GET …/messages` | caller token |
| `POST /v1/agent/calls/{id}/messages`, `GET …/messages` | assigned agent |
| `POST /v1/public/widgets/{key}/callbacks`, `POST /v1/public/calls/{id}/callback` | public (rate-limited) |
| `GET /v1/callbacks`, `GET /v1/callbacks/{id}` | department members (their departments), tenant admin (all) |
| `POST /v1/callbacks/{id}/claim · release · complete · cancel`, `POST /v1/admin/callbacks/{id}/assign` | per §4 |
| `GET /v1/calls`, `GET /v1/calls/{id}` | admin / supervisor / agent, scoped (§5) |
| `POST /v1/calls/{id}/recording/access`, `DELETE /v1/calls/{id}/recording` | admin, supervisor (access only) |
| `GET /v1/platform/recordings/deleted`, `POST …/{id}/access`, `POST …/{id}/restore` | platform admin |
| `POST /v1/webhooks/livekit` | existing; now also handles `egress_ended` |

Out-of-scope ids return 404, never 403 (no existence leak), as in M3. Request bodies are dedicated DTOs with `Validate()`.

## 8. Backend structure

```text
internal/
  storage/     Store interface + minio-go implementation (presign, ensure bucket)
  recording/   model, repository, service (start, egress events, access, soft delete, restore, sweep), handler
  chat/        service, repository, handler (caller and agent endpoints)
  callback/    model, dto, repository, service (workflow, notifications), handler
  history/     read model: list/detail with role scoping, handler
  media/       + StartRecording/StopRecording, egress webhook events, SendData
  routing/     Accept calls a small Recorder interface after commit (no import of recording)
```
Same dependency rule as M3: `routing` and `call` depend on narrow interfaces defined at the point of use, wired in `app`.

## 9. Frontend

```text
web/app/calls/            page.tsx (list + filters), [id]/page.tsx (detail: timeline, recording player, transcript, delete), actions.ts
web/app/callbacks/        page.tsx (list), [id]/page.tsx (claim / release / complete), actions.ts
web/app/platform/recordings/deleted/   list, play, restore
web/app/agent/            AgentConsole: chat panel + "callbacks waiting" badge
web/app/widget/[publicKey]/  ChatPanel, CallbackForm, "recorded" notice; closed / no-agents / missed show the callback form
web/lib/chat/             useChat (LiveKit DataReceived + history fetch + send, de-duplicated by id)
```
- **Proposal:** one `/calls` and one `/callbacks` area for all tenant roles, scoped by the API (admins see all, supervisors/agents their share), instead of v0.4's `/admin/calls`, `/supervisor/calls`, `/agent/calls`, `/admin/callbacks`, `/agent/callbacks`. Same pages, same components, no duplicated code. Navigation entries are added to the workspace and admin sidebars. If you prefer per-role URLs they can be thin route re-exports.
- Every list/detail page has loading, empty, error and success states. The recording player requests the presigned URL on demand (button), never embeds it in the page HTML.
- Widget: while `active` the caller sees "This call is being recorded" and the chat panel. Chat messages render as text, with the agent's and caller's messages visually distinct.

## 10. Failure & edge cases

| Case | Behaviour |
|------|-----------|
| Egress cannot start | Recording `failed` with the error; call continues; admin detail shows "unavailable"; agent console shows a warning. |
| Egress finishes but no webhook arrives | Sweeper marks it `failed` after the grace period. The object may exist in storage; an operator can reconcile (not automated in M4). |
| Duplicate/replayed `egress_ended` | Conditional update; second delivery changes nothing. |
| Webhook arrives before the recording row has its egress id | Handler answers 5xx so LiveKit retries (narrow race). |
| Call ends before egress is up | Egress start fails or yields an empty file → `failed`. |
| Presigned URL leaked | Valid 5 min, one object; every issuance is audit-logged. |
| Chat message when call not `active` / not a participant | 409 / 404, nothing stored. |
| Chat persisted but `SendData` fails | Message is saved (transcript is complete); the sender sees it, the other side gets it on the next history fetch. Logged. |
| Two agents claim the same callback | Conditional update: one wins, the other gets `409 callback_taken`. |
| Callback spam from one IP | Rate limit; nothing else stored beyond the row. |
| Tenant deleted | Rows cascade; **storage objects are not deleted automatically** (called out; cleanup job is M6). |
| Soft-deleted recording of a deleted tenant | Gone with the tenant rows; object cleanup as above. |

## 11. Security

- Recording access is authorised in the service by role and department scope, then audit-logged; storage keys and presigned URLs never appear in lists, logs or the page HTML.
- The bucket is private; MinIO credentials live only in API/egress configuration, never in the browser.
- Public callback and message endpoints are rate-limited and validated like M2's call creation; messages and callback text are plain text and escaped on output; the callback email to admins is built from escaped text.
- Webhooks stay signature-verified; egress events are applied idempotently.
- Tenant isolation: RLS and composite FKs on all three new tables, covered by extending the existing isolation tests.

## 12. Testing

| Level | What |
|-------|------|
| Unit | Callback and chat DTO validation; recording state transitions; scope rules for history; presign options. |
| Integration (Postgres) | Callback claim race (parallel claims, one winner), workflow permissions, history scoping per role and tenant, soft delete hides from tenant queries but not from platform/quota sums, restore, sweeper marks stale recordings, chat only when active, RLS on the new tables. Storage and media are faked. |
| API smoke (`smoke_m4.py`) | Full HTTP flow with real LiveKit and MinIO: accept → recording `recording`, chat both ways with persistence, end → recording `ready`, presigned playback returns bytes, soft delete → hidden → platform restore; missed call → callback → claim → complete; closed/no-agents callbacks. |
| Media | The spike (§2.1) and an egress run driven by the LiveKit CLI. |
| Manual | Real audio with a real microphone: listen to the recording; confirm both voices are present and the opening seconds are not lost. |

## 13. Changes to v0.5 (need your OK)

| # | Earlier text | M4 proposes | Why |
|---|--------------|-------------|-----|
| C1 | Redis not used (D26) | Redis added to the dev stack **for LiveKit Egress only** | Egress requires it; the API still does not use Redis. |
| C2 | Recording starts on `participant_joined` (§5) | Starts at **accept** | Head start for egress startup, no dependence on webhook timing. |
| C3 | `recordings.status`: recording / processing / ready / failed | `starting` / `recording` / `ready` / `failed` | `processing` has no observable meaning with LiveKit; `starting` covers the window before an egress id exists. |
| C4 | Pages `/admin/calls`, `/supervisor/calls`, `/agent/calls`, `/admin/callbacks`, `/agent/callbacks` | One `/calls` and one `/callbacks` area, scoped by role in the API | No duplicated pages or logic. |
| C5 | Callback notices over WebSocket | Realtime hint over SSE (D25) + admin email | Consistent with M3. |
| C6 | Packages `recording`, `chat`, `callback` | Adds `storage` and `history` | Single responsibility; the history read model does not belong in `call`. |

## 14. Assumptions

1. Supervisors may play recordings and read transcripts for their departments, audit-logged (v0.4 §13.1). Agents may not play recordings.
2. No retention policy or automatic deletion in M4.
3. One admin email per new callback.
4. Callbacks are returned outside the platform.
5. A recording failure does not block or end the call (v0.4 §5.3).
6. Mixed single-file composite recording is acceptable (§2.2).

## 15. Open questions

1. **C1 — Add Redis (for LiveKit Egress only)?** There is no way to record with LiveKit without it. Recommended: yes.
2. **Should agents be able to play recordings of calls they handled?** Recommended: **no** (history and transcript only); supervisors and admins can. Say yes if agents need it for self-review.
3. **C4 — One `/calls` and `/callbacks` area for all roles** instead of per-role URLs? Recommended: yes.
4. **Composite (one mixed file, heavier) or per-participant tracks (cheaper, needs mixing)?** Recommended: composite.

## 16. Implementation order (backend first, riskiest first)

1. **Spike:** compose with redis/minio/egress; record one LiveKit CLI call into MinIO on this machine; confirm file, size and the network approach in §2.1.
2. Migration `00007` + RLS/isolation tests.
3. `storage`, `media` extensions, `recording` (start, egress events, sweeper, access, soft delete, restore) + platform endpoints.
4. `chat`.
5. `callback` (public endpoints, workflow, notifications).
6. `history` read model and endpoints.
7. Backend tests, `smoke_m4.py`.
8. Frontend: shared chat hook → widget (chat, recorded notice, callback form) → agent console (chat, callbacks badge) → `/callbacks` → `/calls` → platform deleted recordings.
9. Fold accepted changes into `ARCHITECTURE.md` (→ v0.6), update the README.

## 17. Implementation notes

Approved with all four open questions answered as recommended (Redis for the recorder only, agents cannot play recordings, one `/calls` and `/callbacks` area, mixed composite recording). Differences and findings:

- **Spike result (§2.1):** sharing LiveKit's network namespace was not enough. The recorder's browser could not complete ICE to the advertised loopback address. LiveKit now advertises both loopback (for browsers and CLIs on the host) and the Docker bridge range (for the recorder): `rtc.ips.includes` in `deploy/livekit/livekit.yaml`, replacing `node_ip`. With that, a composite recording runs end to end into MinIO (about 64 kbit/s, roughly 0.5 MB per minute). `disable_manifest` stops the recorder writing a stray `EG_<id>.json` beside each file.
- **The recorder takes 5–8 seconds to start** (it launches a headless browser; measured 5 s idle, about 8 s on a busy machine). Because recording starts at accept (C2), **the first seconds of an answered call are not captured**. This is the main open quality issue; see "Recommended follow-up" below.
- **Slow-start failure mode:** the recorder gives up after about 20 seconds without a start signal ("Start signal not received"). A busy host (the dev machine at load average 10+) hit this. The recording is then `failed` with that reason and the call is unaffected, as designed.
- Unknown egress ids in `egress_ended` webhooks are **logged and ignored** instead of answered with 5xx (the planned retry): the report arrives seconds after the row exists, so the race it guarded against does not occur.
- The unfinished-recording sweeper is a loop inside `recording.Service` (every 30 s), not part of the routing sweeper.
- `callback_requests` has a unique index on `call_id`: one callback per missed call.
- `session.open_callbacks` feeds the agent console; callback creation and every workflow step publish a hint to the department's members.
- The department filter on the call list is offered to administrators only (only they can list departments); supervisors and agents already see just their own calls. Date filters are calendar days in UTC.
- The in-call chat and recording notice on the widget and the agent console were verified through the API and the type/lint/build checks; they were **not** exercised in a browser (no microphone in the in-app browser).

### Recommended follow-up: start recording when the call starts ringing
Starting the recorder when the first offer is made (not at accept) would overlap its startup with the ringing time (up to the 20 s offer timeout), so the opening words are captured. Cost: a recorder runs for every call that reaches ringing, including declined or missed ones, and the recording then also contains the caller's side of the time between ringing and answer. That changes the earlier rule "ringing time is never recorded", so it needs your decision.
