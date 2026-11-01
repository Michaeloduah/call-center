import type { Metadata } from "next";
import Link from "next/link";

import { setSupervisorsAction } from "@/app/admin/departments/actions";
import { UserPicker } from "@/app/admin/departments/UserPicker";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { usersApi } from "@/lib/api/users";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Department supervisors · CallCenter" };

export default async function DepartmentSupervisorsPage({ params }: PageProps<"/admin/departments/[id]/supervisors">) {
  await requireRole("tenant_admin");
  const { id } = await params;
  const [d, supervisors] = await Promise.all([orNotFound(departmentsApi.get(id)), usersApi.listAll({ role: "supervisor" })]);
  const eligible = supervisors.filter((u) => u.status !== "disabled");
  return (
    <>
      <PageHeader
        title={`Supervisors · ${d.name}`}
        description="Supervisors see the live board and call history only for the departments assigned here."
        back={{ href: `/admin/departments/${d.id}`, label: d.name }}
      />
      <Card>
        <UserPicker
          users={eligible}
          selected={d.supervisors.map((s) => s.id)}
          action={setSupervisorsAction.bind(null, d.id)}
          cancelHref={`/admin/departments/${d.id}`}
          emptyMessage="No users have the Supervisor role yet."
        />
        {eligible.length === 0 ? (
          <p className="mt-4 text-sm">
            <Link href="/admin/users/new" className="font-medium text-indigo-600 hover:text-indigo-500">
              Invite a supervisor
            </Link>
          </p>
        ) : null}
      </Card>
    </>
  );
}
