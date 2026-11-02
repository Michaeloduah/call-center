import type { Metadata } from "next";

import { setBusinessHoursAction } from "@/app/admin/departments/actions";
import { HoursEditor } from "@/app/admin/departments/HoursEditor";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Business hours · CallCenter" };

export default async function DepartmentHoursPage({ params }: PageProps<"/admin/departments/[id]/hours">) {
  await requireRole("tenant_admin");
  const { id } = await params;
  const d = await orNotFound(departmentsApi.get(id));
  return (
    <>
      <PageHeader title={`Business hours · ${d.name}`} back={{ href: `/admin/departments/${d.id}`, label: d.name }} />
      <Card>
        <HoursEditor
          initial={d.business_hours}
          timezone={d.timezone}
          action={setBusinessHoursAction.bind(null, d.id)}
          cancelHref={`/admin/departments/${d.id}`}
        />
      </Card>
    </>
  );
}
