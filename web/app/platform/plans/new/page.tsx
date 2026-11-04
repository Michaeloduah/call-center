import type { Metadata } from "next";

import { PlanForm } from "@/app/platform/plans/PlanForms";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "New plan · CallCenter" };

export default async function NewPlanPage() {
  await requireRole("platform_admin");
  return (
    <>
      <PageHeader title="New plan" back={{ href: "/platform/plans", label: "Plans" }} />
      <Card>
        <PlanForm />
      </Card>
    </>
  );
}
