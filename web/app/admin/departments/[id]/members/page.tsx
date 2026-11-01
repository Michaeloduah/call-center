import type { Metadata } from "next";

import { setMembersAction } from "@/app/admin/departments/actions";
import { UserPicker } from "@/app/admin/departments/UserPicker";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { usersApi } from "@/lib/api/users";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Department agents · CallCenter" };

export default async function DepartmentMembersPage({ params }: PageProps<"/admin/departments/[id]/members">) {
  await requireRole("tenant_admin");
  const { id } = await params;
  const [d, users] = await Promise.all([orNotFound(departmentsApi.get(id)), usersApi.listAll({})]);
  const eligible = users.filter((u) => u.status !== "disabled");
  return (
    <>
      <PageHeader
        title={`Agents · ${d.name}`}
        description="Choose who takes calls for this department. A user can belong to several departments."
        back={{ href: `/admin/departments/${d.id}`, label: d.name }}
      />
      <Card>
        <UserPicker
          users={eligible}
          selected={d.members.map((m) => m.id)}
          action={setMembersAction.bind(null, d.id)}
          cancelHref={`/admin/departments/${d.id}`}
          emptyMessage="There are no active or invited users yet. Invite users first."
        />
      </Card>
    </>
  );
}
