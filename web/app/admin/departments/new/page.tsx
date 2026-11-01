import type { Metadata } from "next";

import { DepartmentForm } from "@/app/admin/departments/DepartmentForm";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { timezoneOptions } from "@/lib/timezones";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "New department · CallCenter" };

export default async function NewDepartmentPage() {
  await requireRole("tenant_admin");
  return (
    <>
      <PageHeader title="New department" back={{ href: "/admin/departments", label: "Departments" }} />
      <div className="max-w-xl">
        <Card>
          <DepartmentForm timezones={timezoneOptions()} />
        </Card>
      </div>
    </>
  );
}
