import type { Metadata } from "next";

import { EditUserForm } from "@/app/admin/users/UserForms";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { usersApi } from "@/lib/api/users";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Edit user · CallCenter" };

export default async function EditUserPage({ params }: PageProps<"/admin/users/[id]/edit">) {
  const [{ id }, me] = await Promise.all([params, requireRole("tenant_admin")]);
  const user = await orNotFound(usersApi.get(id));
  return (
    <>
      <PageHeader title={`Edit ${user.full_name}`} back={{ href: `/admin/users/${user.id}`, label: user.full_name }} />
      <div className="max-w-xl">
        <Card>
          <EditUserForm user={user} isSelf={me.id === user.id} />
        </Card>
      </div>
    </>
  );
}
