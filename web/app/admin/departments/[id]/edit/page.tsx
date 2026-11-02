import type { Metadata } from "next";

import { DepartmentForm } from "@/app/admin/departments/DepartmentForm";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { orNotFound } from "@/lib/server/api";
import { timezoneOptions } from "@/lib/timezones";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Edit department · CallCenter" };

export default async function EditDepartmentPage({ params }: PageProps<"/admin/departments/[id]/edit">) {
  await requireRole("tenant_admin");
  const { id } = await params;
  const d = await orNotFound(departmentsApi.get(id));
  return (
    <>
      <PageHeader title={`Edit ${d.name}`} back={{ href: `/admin/departments/${d.id}`, label: d.name }} />
      <div className="max-w-xl">
        <Card>
          <DepartmentForm department={d} timezones={timezoneOptions()} />
        </Card>
      </div>
    </>
  );
}
