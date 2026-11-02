import type { Metadata } from "next";

import { CreateUserForm } from "@/app/admin/users/UserForms";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Invite user · CallCenter" };

export default async function NewUserPage() {
  await requireRole("tenant_admin");
  return (
    <>
      <PageHeader title="Invite user" back={{ href: "/admin/users", label: "Users" }} />
      <div className="max-w-xl">
        <Card>
          <CreateUserForm />
        </Card>
      </div>
    </>
  );
}
