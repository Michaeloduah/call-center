import type { Metadata } from "next";
import Link from "next/link";

import { WidgetForm } from "@/app/admin/widgets/WidgetForm";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { departmentsApi } from "@/lib/api/departments";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "New widget · CallCenter" };

export default async function NewWidgetPage() {
  await requireRole("tenant_admin");
  const departments = await departmentsApi.listAll();
  return (
    <>
      <PageHeader title="New widget" back={{ href: "/admin/widgets", label: "Widgets" }} />
      {departments.length === 0 ? (
        <EmptyState
          title="Create a department first"
          description="Every widget routes its calls to a department."
          action={<Link href="/admin/departments/new" className="text-sm font-medium text-indigo-600">New department</Link>}
        />
      ) : (
        <div className="max-w-2xl">
          <Card>
            <WidgetForm departments={departments} />
          </Card>
        </div>
      )}
    </>
  );
}
