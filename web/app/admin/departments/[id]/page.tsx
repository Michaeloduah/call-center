import type { Metadata } from "next";
import Link from "next/link";

import { deleteDepartmentAction } from "@/app/admin/departments/actions";
import { ActionError } from "@/components/ui/ActionError";
import { Badge, UserStatusBadge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { formatDateTime, weekdayLabels } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import type { DepartmentUser, HoursRange } from "@/lib/types";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Department · CallCenter" };

function UserList({ users, empty }: { users: DepartmentUser[]; empty: string }) {
  if (users.length === 0) return <p className="text-sm text-slate-500">{empty}</p>;
  return (
    <ul className="divide-y divide-slate-100">
      {users.map((u) => (
        <li key={u.id} className="flex items-center justify-between py-2 text-sm">
          <Link href={`/admin/users/${u.id}`} className="hover:underline">
            <span className="font-medium text-slate-900">{u.full_name}</span>
            <span className="ml-2 text-slate-500">{u.email}</span>
          </Link>
          <UserStatusBadge status={u.status} />
        </li>
      ))}
    </ul>
  );
}

function HoursSummary({ hours }: { hours: HoursRange[] }) {
  if (hours.length === 0) {
    return <p className="text-sm text-amber-800">No opening hours set — this department is always closed.</p>;
  }
  return (
    <dl className="grid grid-cols-[8rem_1fr] gap-y-1.5 text-sm">
      {weekdayLabels.map((label, day) => {
        const ranges = hours.filter((h) => h.weekday === day);
        return (
          <div key={label} className="contents">
            <dt className="text-slate-500">{label}</dt>
            <dd className="text-slate-900">{ranges.length ? ranges.map((r) => `${r.opens}–${r.closes}`).join(", ") : "Closed"}</dd>
          </div>
        );
      })}
    </dl>
  );
}

export default async function DepartmentDetailPage({ params, searchParams }: PageProps<"/admin/departments/[id]">) {
  await requireRole("tenant_admin");
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  const d = await orNotFound(departmentsApi.get(id));
  const base = `/admin/departments/${d.id}`;

  return (
    <>
      <PageHeader
        title={d.name}
        description={d.description || undefined}
        back={{ href: "/admin/departments", label: "Departments" }}
        actions={
          <>
            <LinkButton href={`${base}/edit`} variant="secondary">
              Edit
            </LinkButton>
            <ConfirmButton
              action={deleteDepartmentAction.bind(null, d.id)}
              confirmMessage={`Delete the ${d.name} department? Agent and supervisor assignments will be removed.`}
              variant="danger"
              pendingLabel="Deleting…"
            >
              Delete
            </ConfirmButton>
          </>
        }
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />
      <div className="space-y-6">
        <Card title="Details">
          <DescriptionList
            items={[
              { label: "Status", value: d.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge> },
              { label: "Timezone", value: d.timezone },
              { label: "Created", value: formatDateTime(d.created_at) },
              { label: "Last updated", value: formatDateTime(d.updated_at) },
            ]}
          />
        </Card>
        <Card title={`Agents (${d.members.length})`} actions={<LinkButton href={`${base}/members`} variant="secondary">Manage</LinkButton>}>
          <UserList users={d.members} empty="No agents assigned. Calls to this department cannot be answered yet." />
        </Card>
        <Card
          title={`Supervisors (${d.supervisors.length})`}
          actions={<LinkButton href={`${base}/supervisors`} variant="secondary">Manage</LinkButton>}
        >
          <UserList users={d.supervisors} empty="No supervisors assigned." />
        </Card>
        <Card title="Business hours" actions={<LinkButton href={`${base}/hours`} variant="secondary">Edit</LinkButton>}>
          <HoursSummary hours={d.business_hours} />
        </Card>
      </div>
    </>
  );
}
