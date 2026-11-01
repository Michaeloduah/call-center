import { tenantStatusLabels, userStatusLabels } from "@/lib/format";
import type { TenantStatus, UserStatus } from "@/lib/types";

const tones = {
  green: "bg-green-50 text-green-700 ring-green-600/20",
  amber: "bg-amber-50 text-amber-800 ring-amber-600/20",
  red: "bg-red-50 text-red-700 ring-red-600/20",
  slate: "bg-slate-50 text-slate-700 ring-slate-500/20",
  indigo: "bg-indigo-50 text-indigo-700 ring-indigo-600/20",
};

export function Badge({ tone = "slate", children }: { tone?: keyof typeof tones; children: React.ReactNode }) {
  return (
    <span className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${tones[tone]}`}>
      {children}
    </span>
  );
}

const userTone: Record<UserStatus, keyof typeof tones> = { active: "green", invited: "amber", disabled: "red" };
const tenantTone: Record<TenantStatus, keyof typeof tones> = { active: "green", pending_verification: "amber", suspended: "red" };

export function UserStatusBadge({ status }: { status: UserStatus }) {
  return <Badge tone={userTone[status]}>{userStatusLabels[status]}</Badge>;
}

export function TenantStatusBadge({ status }: { status: TenantStatus }) {
  return <Badge tone={tenantTone[status]}>{tenantStatusLabels[status]}</Badge>;
}
