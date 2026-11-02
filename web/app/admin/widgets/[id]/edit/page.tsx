import type { Metadata } from "next";

import { WidgetForm } from "@/app/admin/widgets/WidgetForm";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { widgetsApi } from "@/lib/api/widgets";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Edit widget · CallCenter" };

export default async function EditWidgetPage({ params }: PageProps<"/admin/widgets/[id]/edit">) {
  await requireRole("tenant_admin");
  const { id } = await params;
  const [widget, departments] = await Promise.all([orNotFound(widgetsApi.get(id)), departmentsApi.listAll()]);
  return (
    <>
      <PageHeader title={`Edit ${widget.name}`} back={{ href: `/admin/widgets/${widget.id}`, label: widget.name }} />
      <div className="max-w-2xl">
        <Card>
          <WidgetForm widget={widget} departments={departments} />
        </Card>
      </div>
    </>
  );
}
