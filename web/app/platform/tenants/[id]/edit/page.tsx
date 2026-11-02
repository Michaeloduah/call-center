import type { Metadata } from "next";

import { EditTenantForm } from "@/app/platform/tenants/TenantForms";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { tenantsApi } from "@/lib/api/tenants";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Edit tenant · CallCenter" };

export default async function EditTenantPage({ params }: PageProps<"/platform/tenants/[id]/edit">) {
  await requireRole("platform_admin");
  const { id } = await params;
  const tenant = await orNotFound(tenantsApi.get(id));
  return (
    <>
      <PageHeader title={`Edit ${tenant.name}`} back={{ href: `/platform/tenants/${tenant.id}`, label: tenant.name }} />
      <div className="max-w-xl">
        <Card>
          <EditTenantForm tenant={tenant} />
        </Card>
      </div>
    </>
  );
}
