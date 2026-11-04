import type { Metadata } from "next";

import { PlanForm } from "@/app/platform/plans/PlanForms";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { plansApi } from "@/lib/api/billing";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Edit plan · CallCenter" };

export default async function EditPlanPage({ params }: PageProps<"/platform/plans/[id]/edit">) {
  await requireRole("platform_admin");
  const { id } = await params;
  const plan = await orNotFound(plansApi.get(id));
  return (
    <>
      <PageHeader title={`Edit ${plan.name}`} back={{ href: `/platform/plans/${plan.id}`, label: plan.name }} description="Changes apply to every subscriber straight away." />
      <Card>
        <PlanForm plan={plan} />
      </Card>
    </>
  );
}
