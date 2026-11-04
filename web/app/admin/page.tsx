import type { Metadata } from "next";
import Link from "next/link";

import { Alert } from "@/components/ui/Alert";
import { LinkButton } from "@/components/ui/Button";
import { PageHeader } from "@/components/ui/PageHeader";
import { billingApi } from "@/lib/api/billing";
import { departmentsApi } from "@/lib/api/departments";
import { usersApi } from "@/lib/api/users";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Overview · CallCenter" };

function Stat({ label, value, href }: { label: string; value: number; href: string }) {
  return (
    <Link href={href} className="rounded-lg bg-white p-5 shadow-sm ring-1 ring-slate-200 hover:ring-indigo-300">
      <p className="text-sm text-slate-500">{label}</p>
      <p className="mt-2 text-3xl font-semibold text-slate-900">{value}</p>
    </Link>
  );
}

export default async function AdminOverviewPage() {
  const user = await requireRole("tenant_admin");
  // per_page=1: only the totals are needed.
  const [users, agents, invited, departments, billing] = await Promise.all([
    usersApi.list({ per_page: 1 }),
    usersApi.list({ role: "agent", per_page: 1 }),
    usersApi.list({ status: "invited", per_page: 1 }),
    departmentsApi.list({}),
    billingApi.overview(),
  ]);
  const { usage, limits } = billing;
  const nearCap = (used: number, cap: number | null) => cap !== null && cap > 0 && used >= cap * 0.8;
  const warnings = [
    billing.subscription.status === "past_due" ? "Your last payment failed." : null,
    nearCap(usage.call_seconds, limits.max_call_minutes_per_month === null ? null : limits.max_call_minutes_per_month * 60) ? "You have used most of this month's call minutes." : null,
    nearCap(usage.storage_bytes, limits.max_storage_bytes) ? "Your recording storage is almost full." : null,
    billing.agents_without_seat.length > 0 ? "Some agents have no seat on your plan." : null,
  ].filter((w): w is string => w !== null);

  return (
    <>
      <PageHeader title={user.tenant?.name ?? "Overview"} description="Your team and departments at a glance." />
      {warnings.length > 0 ? (
        <div className="mb-6">
          <Alert tone="warning">
            {warnings.join(" ")}{" "}
            <Link href="/admin/billing" className="font-medium underline">
              Open billing
            </Link>
          </Alert>
        </div>
      ) : null}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Users" value={users.total} href="/admin/users" />
        <Stat label="Agents" value={agents.total} href="/admin/users?role=agent" />
        <Stat label="Pending invitations" value={invited.total} href="/admin/users?status=invited" />
        <Stat label="Departments" value={departments.total} href="/admin/departments" />
      </div>
      {departments.total === 0 ? (
        <div className="mt-8 rounded-lg bg-indigo-50 p-5 ring-1 ring-indigo-100">
          <h2 className="text-sm font-semibold text-indigo-900">Get started</h2>
          <p className="mt-1 text-sm text-indigo-800">
            Create a department, invite your agents, then assign them to it. Calls are routed to a department&apos;s agents.
          </p>
          <div className="mt-4 flex gap-2">
            <LinkButton href="/admin/departments/new">Create a department</LinkButton>
            <LinkButton href="/admin/users/new" variant="secondary">
              Invite a user
            </LinkButton>
          </div>
        </div>
      ) : null}
    </>
  );
}
