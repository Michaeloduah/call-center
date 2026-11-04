"use server";

import { createHmac, randomUUID } from "node:crypto";

import { notFound, redirect } from "next/navigation";

import { requireRole } from "@/lib/server/session";

/** The simulated provider is only ever available in development. */
function fakeSecret(): string {
  if (process.env.BILLING_FAKE !== "true" || !process.env.BILLING_FAKE_SECRET) notFound();
  return process.env.BILLING_FAKE_SECRET;
}

/** Sends the API a signed webhook, like a real payment provider would after a payment. */
export async function simulatePaymentAction(tenantId: string, priceId: string, succeed: boolean): Promise<void> {
  const secret = fakeSecret();
  const user = await requireRole("tenant_admin");
  // A tenant can only simulate a payment for itself.
  if (user.tenant?.id !== tenantId) notFound();

  const id = randomUUID();
  const payload = succeed
    ? {
        id: `evt_${id}`, type: "subscription.activated", tenant_id: tenantId, plan_price_id: priceId,
        external_customer_id: `cus_${tenantId.slice(0, 8)}`, external_subscription_id: `sub_${id.slice(0, 12)}`,
        authorization: "fake_authorization", payment_reference: `pay_${id}`,
      }
    : { id: `evt_${id}`, type: "payment.failed", tenant_id: tenantId, payment_reference: `pf_${id}`, failure: "Simulated card decline" };
  const body = JSON.stringify(payload);
  const res = await fetch(new URL("/v1/webhooks/fake", process.env.API_BASE_URL), {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Fake-Signature": createHmac("sha256", secret).update(body).digest("hex") },
    body,
    cache: "no-store",
  });
  if (!res.ok) redirect("/admin/billing?error=provider_unavailable");
  redirect("/admin/billing?checkout=return");
}
