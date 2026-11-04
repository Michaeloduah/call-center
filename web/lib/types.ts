// Shapes returned by the Go API. Keep in sync with the backend JSON tags.

export type Role = "platform_admin" | "tenant_admin" | "supervisor" | "agent";
export type TenantRole = Exclude<Role, "platform_admin">;
export type UserStatus = "invited" | "active" | "disabled";
export type TenantStatus = "pending_verification" | "active" | "suspended";

export interface Paginated<T> {
  items: T[];
  page: number;
  per_page: number;
  total: number;
}

export interface Me {
  id: string;
  email: string;
  full_name: string;
  role: Role;
  tenant: { id: string; name: string } | null;
}

export interface LoginResult {
  session_token: string;
  expires_at: string;
  user: Me;
}

export interface SignupResult {
  email: string;
  email_sent: boolean;
}

export interface InviteInfo {
  email: string;
  full_name: string;
  tenant_name: string | null;
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  status: TenantStatus;
  created_at: string;
  updated_at: string;
}

export interface TenantSummary extends Tenant {
  users_count: number;
}

export interface TenantDetail extends Tenant {
  users_count: number;
  departments_count: number;
  admins: { id: string; full_name: string; email: string; status: UserStatus }[];
}

export interface TenantCreateResult {
  tenant: Tenant;
  invite_email_sent: boolean;
}

export interface User {
  id: string;
  email: string;
  full_name: string;
  role: TenantRole;
  status: UserStatus;
  email_verified_at: string | null;
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface DepartmentRef {
  id: string;
  name: string;
}

export interface UserDetail extends User {
  departments: DepartmentRef[];
  supervised_departments: DepartmentRef[];
}

export interface InviteResult {
  user: User;
  invite_email_sent: boolean;
}

export interface Department {
  id: string;
  name: string;
  description: string;
  timezone: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface DepartmentSummary extends Department {
  members_count: number;
  supervisors_count: number;
}

export interface DepartmentUser {
  id: string;
  full_name: string;
  email: string;
  role: TenantRole;
  status: UserStatus;
}

export interface HoursRange {
  weekday: number;
  opens: string;
  closes: string;
}

export interface DepartmentDetail extends Department {
  members: DepartmentUser[];
  supervisors: DepartmentUser[];
  business_hours: HoursRange[];
}

export interface PlatformSettings {
  default_consent_text: string;
  updated_at: string;
}

export interface TenantSettings {
  offer_timeout_seconds: number;
  max_queue_wait_seconds: number;
  consent_text_override: string | null;
  default_consent_text: string;
  effective_consent_text: string;
  updated_at: string;
}

export interface AuditLogEntry {
  id: string;
  tenant_id: string | null;
  tenant_name: string | null;
  actor_user_id: string | null;
  actor_email: string | null;
  action: string;
  entity_type: string;
  entity_id: string | null;
  metadata: Record<string, unknown>;
  ip: string | null;
  created_at: string;
}

export type FieldMode = "hidden" | "optional" | "required";

export interface CallerForm {
  enabled: boolean;
  name: FieldMode;
  phone: FieldMode;
  email: FieldMode;
}

export interface Widget {
  id: string;
  department_id: string;
  department_name: string;
  name: string;
  public_key: string;
  allowed_origins: string[];
  is_active: boolean;
  button_label: string;
  accent_color: string;
  greeting: string;
  caller_form: CallerForm;
  created_at: string;
  updated_at: string;
}

export type Availability = "open" | "closed" | "unavailable" | "no_agents" | "limit_reached";

/** Caller-facing widget configuration (public, contains nothing secret). */
export interface PublicWidgetConfig {
  public_key: string;
  tenant_name: string;
  department_name: string;
  button_label: string;
  accent_color: string;
  greeting: string;
  caller_form: CallerForm;
  consent_text: string;
  availability: Availability;
  timezone: string;
  business_hours: HoursRange[];
  frame_ancestors: string[];
}

export type CallStatus = "queued" | "ringing" | "active" | "completed" | "abandoned" | "missed" | "failed";

export interface CallCreateResult {
  call_id: string;
  caller_token: string;
  expires_at: string;
  status: CallStatus;
  media_url: string;
  media_token: string;
}

export interface CallerCallView {
  id: string;
  status: CallStatus;
  queue_position: number | null;
  end_reason: string | null;
}

// ── Agent console & live board ──────────────────────────────────────────────

/** What an agent chooses. "Ringing" and "on a call" are derived by the API, never chosen. */
export type AgentStatus = "offline" | "available" | "away";
export type AgentState = "offline" | "away" | "idle" | "ringing" | "on_call";

/** A ringing call. Only the caller's name is shown until the agent accepts. */
export interface Offer {
  id: string;
  call_id: string;
  department_name: string;
  caller_name: string | null;
  expires_at: string;
}

export interface ActiveCall {
  id: string;
  department_name: string;
  caller_name: string | null;
  caller_phone: string | null;
  caller_email: string | null;
  answered_at: string;
}

export interface AgentSession {
  status: AgentStatus;
  state: AgentState;
  offer: Offer | null;
  call: ActiveCall | null;
  open_callbacks: number;
  heartbeat_interval_seconds: number;
}

export interface AcceptResult {
  call: ActiveCall;
  media_url: string;
  media_token: string;
}

export interface MediaGrant {
  media_url: string;
  media_token: string;
}

export interface BoardAgent {
  id: string;
  full_name: string;
  state: AgentState;
}

export interface BoardCall {
  id: string;
  status: "ringing" | "active";
  caller_name: string | null;
  agent_name: string | null;
  since: string;
}

export interface BoardDepartment {
  id: string;
  name: string;
  queued: number;
  oldest_queued_since: string | null;
  agents: BoardAgent[];
  calls: BoardCall[];
}

export interface Board {
  departments: BoardDepartment[];
}

// ── Recordings, chat, callbacks & call history ──────────────────────────────

export type RecordingStatus = "starting" | "recording" | "ready" | "failed";

export interface RecordingView {
  id: string;
  status: RecordingStatus;
  duration_seconds: number | null;
  size_bytes: number | null;
  started_at: string;
  ended_at: string | null;
  error: string | null;
}

/** A short-lived link to a recording. Treat it like a password: never store or log it. */
export interface AccessGrant {
  url: string;
  expires_at: string;
}

export interface DeletedRecording {
  id: string;
  call_id: string;
  tenant_id: string;
  tenant_name: string;
  department_name: string;
  caller_name: string | null;
  duration_seconds: number | null;
  size_bytes: number | null;
  deleted_at: string;
  deleted_by_email: string | null;
}

export interface ChatMessage {
  id: string;
  sender: "caller" | "agent";
  name: string;
  body: string;
  sent_at: string;
}

export type CallRecordStatus = CallStatus;

export interface CallListItem {
  id: string;
  status: CallStatus;
  end_reason: string | null;
  department_id: string;
  department_name: string;
  caller_name: string | null;
  caller_phone: string | null;
  agent_name: string | null;
  queued_at: string;
  answered_at: string | null;
  ended_at: string | null;
  duration_seconds: number | null;
  recording_status: RecordingStatus | null;
  message_count: number;
}

export interface OfferEntry {
  agent_name: string | null;
  offered_at: string;
  responded_at: string | null;
  outcome: "pending" | "accepted" | "declined" | "timeout" | "cancelled";
}

export interface CallDetail extends CallListItem {
  caller_email: string | null;
  consent_text: string;
  consent_given_at: string;
  offers: OfferEntry[];
  recording: RecordingView | null;
  messages: ChatMessage[];
  callback: { id: string; status: CallbackStatus } | null;
  permissions: { play_recording: boolean; delete_recording: boolean };
}

export type CallbackStatus = "open" | "claimed" | "completed" | "cancelled";

export interface Callback {
  id: string;
  department_id: string;
  department_name: string;
  call_id: string | null;
  name: string;
  phone: string | null;
  email: string | null;
  reason: string;
  status: CallbackStatus;
  claimed_by: string | null;
  claimed_by_name: string | null;
  claimed_at: string | null;
  resolution_notes: string;
  resolved_at: string | null;
  created_at: string;
}

// ── Plans, subscriptions & billing ───────────────────────────────────────────

export type PlanInterval = "monthly" | "yearly";
export type SubscriptionStatus = "active" | "past_due" | "cancelled" | "incomplete";
export type CapPolicy = "block" | "overage";

/** A plan's limits. null means unlimited. */
export interface PlanLimits {
  max_agents: number | null;
  max_call_minutes_per_month: number | null;
  max_storage_bytes: number | null;
  max_departments: number | null;
  max_widgets: number | null;
}

export interface PlanPrice {
  id: string;
  plan_id: string;
  currency: string;
  interval: PlanInterval;
  amount_minor: number;
  overage_per_minute_minor: number;
  overage_per_gb_minor: number;
  provider: "paystack" | "stripe" | "fake";
  synced: boolean;
  is_active: boolean;
}

export interface Plan extends PlanLimits {
  id: string;
  code: string;
  name: string;
  description: string;
  is_free: boolean;
  is_public: boolean;
  is_active: boolean;
  prices: PlanPrice[];
  created_at: string;
  updated_at: string;
}

export interface Subscription {
  id: string;
  tenant_id: string;
  plan_id: string;
  plan_price_id: string | null;
  provider: string | null;
  status: SubscriptionStatus;
  past_due_since: string | null;
  cancel_at_period_end: boolean;
  current_period_start: string;
  current_period_end: string;
}

export interface Usage {
  agents: number;
  departments: number;
  widgets: number;
  call_seconds: number;
  storage_bytes: number;
}

export interface Payment {
  id: string;
  kind: "subscription" | "overage";
  provider: string;
  reference: string;
  amount_minor: number;
  currency: string;
  status: "succeeded" | "failed" | "refunded";
  failure: string | null;
  paid_at: string;
}

export interface BillingOverview {
  subscription: Subscription;
  plan: Plan;
  price: PlanPrice | null;
  limits: PlanLimits;
  policies: { minutes_cap_policy: CapPolicy; storage_cap_policy: CapPolicy };
  usage: Usage;
  agents_without_seat: { id: string; full_name: string }[];
  payments: Payment[];
  plans: Plan[];
}

export interface PlatformTenantBilling {
  subscription: Subscription;
  plan: Plan;
  price: PlanPrice | null;
  usage: Usage;
  payments: Payment[];
}
