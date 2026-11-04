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

export type Availability = "open" | "closed" | "unavailable" | "no_agents";

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
