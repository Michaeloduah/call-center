import type {
  AgentState,
  CallStatus,
  CallbackStatus,
  RecordingStatus,
  Role,
  SubscriptionStatus,
  TenantStatus,
  UserStatus,
} from "@/lib/types";

export const roleLabels: Record<Role, string> = {
  platform_admin: "Platform admin",
  tenant_admin: "Admin",
  supervisor: "Supervisor",
  agent: "Agent",
};

export const userStatusLabels: Record<UserStatus, string> = {
  invited: "Invited",
  active: "Active",
  disabled: "Disabled",
};

export const tenantStatusLabels: Record<TenantStatus, string> = {
  pending_verification: "Pending verification",
  active: "Active",
  suspended: "Suspended",
};

export const weekdayLabels = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

const dateTime = new Intl.DateTimeFormat("en-GB", { dateStyle: "medium", timeStyle: "short" });
const dateOnly = new Intl.DateTimeFormat("en-GB", { dateStyle: "medium" });

export function formatDateTime(iso: string | null): string {
  return iso ? dateTime.format(new Date(iso)) : "—";
}

export function formatDate(iso: string | null): string {
  return iso ? dateOnly.format(new Date(iso)) : "—";
}

/** Reads a single string from Next.js searchParams. */
export function param(value: string | string[] | undefined): string {
  return Array.isArray(value) ? (value[0] ?? "") : (value ?? "");
}

export function pageParam(value: string | string[] | undefined): number {
  const n = Number.parseInt(param(value), 10);
  return Number.isFinite(n) && n > 0 ? n : 1;
}

/** 75 → "1:15", 3725 → "1:02:05". Negative values count as zero. */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

export const agentStateLabels: Record<AgentState, string> = {
  offline: "Offline",
  away: "Away",
  idle: "Available",
  ringing: "Ringing",
  on_call: "On a call",
};

export function formatBytes(bytes: number | null): string {
  if (bytes === null) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export const callStatusLabels: Record<CallStatus, string> = {
  queued: "Waiting",
  ringing: "Ringing",
  active: "In progress",
  completed: "Completed",
  abandoned: "Caller left",
  missed: "Missed",
  failed: "Failed",
};

export const endReasonLabels: Record<string, string> = {
  caller_hung_up: "Caller hung up",
  caller_left: "Caller left",
  agent_ended: "Agent ended the call",
  agent_left: "Agent disconnected",
  max_wait_exceeded: "Nobody answered in time",
  room_closed: "Room closed",
  media_unavailable: "Could not connect audio",
};

export const callbackStatusLabels: Record<CallbackStatus, string> = {
  open: "Open",
  claimed: "Claimed",
  completed: "Completed",
  cancelled: "Cancelled",
};

export const recordingStatusLabels: Record<RecordingStatus, string> = {
  starting: "Starting",
  recording: "Recording",
  ready: "Available",
  failed: "Unavailable",
};

/** Decimal digits a currency uses (2 for USD, 0 for JPY), from the platform's Intl data. */
function currencyDigits(currency: string): number {
  return new Intl.NumberFormat("en", { style: "currency", currency }).resolvedOptions().maximumFractionDigits ?? 2;
}

/** 4900, "USD" → "$49.00". Amounts are always integers in the currency's smallest unit. */
export function formatMoney(minor: number, currency: string): string {
  const digits = currencyDigits(currency);
  return new Intl.NumberFormat("en", { style: "currency", currency }).format(minor / 10 ** digits);
}

/** "49.00", "USD" → 4900. Returns null for anything that is not a non-negative number. */
export function toMinor(value: string, currency: string): number | null {
  const trimmed = value.trim();
  if (trimmed === "") return 0;
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.round(n * 10 ** currencyDigits(currency));
}

const GB = 1024 ** 3;

export function formatStorage(bytes: number): string {
  if (bytes >= GB) return `${(bytes / GB).toFixed(bytes >= 10 * GB ? 0 : 1)} GB`;
  return `${Math.round(bytes / 1024 ** 2)} MB`;
}

/** Whole minutes of call time, for usage meters. */
export function formatMinutes(seconds: number): string {
  return `${Math.floor(seconds / 60)} min`;
}

export const subscriptionStatusLabels: Record<SubscriptionStatus, string> = {
  active: "Active",
  past_due: "Payment overdue",
  cancelled: "Cancelled",
  incomplete: "Incomplete",
};
