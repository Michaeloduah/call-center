import type { Role, TenantStatus, UserStatus } from "@/lib/types";

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
