import type { Metadata } from "next";

import { CreateTenantForm } from "@/app/platform/tenants/TenantForms";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "New tenant · CallCenter" };

export default async function NewTenantPage() {
  await requireRole("platform_admin");
  return (
    <>
      <PageHeader title="New tenant" back={{ href: "/platform/tenants", label: "Tenants" }} />
      <div className="max-w-xl">
        <Card>
          <CreateTenantForm />
        </Card>
      </div>
    </>
  );
}
