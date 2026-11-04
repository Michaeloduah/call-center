import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { simulatePaymentAction } from "@/app/admin/billing/fake-checkout/actions";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Simulated payment · CallCenter", robots: { index: false } };

/** Stands in for the payment provider's hosted page while developing. Not available in production. */
export default async function FakeCheckoutPage({ searchParams }: PageProps<"/admin/billing/fake-checkout">) {
  if (process.env.BILLING_FAKE !== "true") notFound();
  await requireRole("tenant_admin");
  const sp = await searchParams;
  const tenantId = param(sp.tenant_id);
  const priceId = param(sp.plan_price_id);
  if (!tenantId || !priceId) notFound();

  return (
    <>
      <PageHeader title="Simulated payment" description="This page replaces the payment provider's checkout while developing." />
      <Card>
        <p className="mb-5 text-sm text-slate-700">Pretend to pay. The provider will then tell the API, exactly as Paystack or Stripe would.</p>
        <div className="flex gap-3">
          <form action={simulatePaymentAction.bind(null, tenantId, priceId, true)}>
            <SubmitButton pendingLabel="Paying…">Pay successfully</SubmitButton>
          </form>
          <form action={simulatePaymentAction.bind(null, tenantId, priceId, false)}>
            <SubmitButton variant="secondary" pendingLabel="Failing…">
              Decline the card
            </SubmitButton>
          </form>
        </div>
      </Card>
    </>
  );
}
