import type { Metadata } from "next";

import { cancelSubscriptionAction, checkoutAction } from "@/app/admin/billing/actions";
import { PoliciesForm } from "@/app/admin/billing/PoliciesForm";
import { UsageMeter } from "@/components/billing/UsageMeter";
import { ActionError } from "@/components/ui/ActionError";
import { Alert } from "@/components/ui/Alert";
import { Badge, SubscriptionStatusBadge } from "@/components/ui/Badge";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { EmptyState } from "@/components/ui/EmptyState";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { Table, Td, Th } from "@/components/ui/Table";
import { billingApi } from "@/lib/api/billing";
import { formatDate, formatDateTime, formatMinutes, formatMoney, formatStorage, param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Billing · CallCenter" };

const plain = (n: number) => String(n);

/** How long a failed payment may stay unpaid before the free plan takes over (D18). */
const GRACE_DAYS = 14;

function graceEnds(pastDueSince: string): string {
  return new Date(new Date(pastDueSince).getTime() + GRACE_DAYS * 86_400_000).toISOString();
}

export default async function BillingPage({ searchParams }: PageProps<"/admin/billing">) {
  await requireRole("tenant_admin");
  const sp = await searchParams;
  const o = await billingApi.overview();
  const { subscription: sub, plan, price, limits, usage, policies } = o;
  const paid = price !== null;
  const returned = param(sp.checkout) === "return" || param(sp.checkout) === "success";

  return (
    <>
      <PageHeader title="Billing" description="Your plan, usage and payments." />
      <Notice value={returned ? "checkout-return" : sp.notice} />
      <ActionError value={sp.error} />

      {sub.status === "past_due" && sub.past_due_since ? (
        <div className="mb-6">
          <Alert tone="error">
            Your last payment failed. Update your payment method before {formatDate(graceEnds(sub.past_due_since))} or your organisation moves to
            the free plan. Nothing is deleted.
          </Alert>
        </div>
      ) : null}
      {o.agents_without_seat.length > 0 ? (
        <div className="mb-6">
          <Alert tone="warning">
            Your plan has {limits.max_agents} agent seat{limits.max_agents === 1 ? "" : "s"} but {usage.agents} agents. These agents cannot go available
            until seats are freed or you upgrade: {o.agents_without_seat.map((a) => a.full_name).join(", ")}.
          </Alert>
        </div>
      ) : null}

      <div className="space-y-6">
        <Card title="Current plan" actions={<SubscriptionStatusBadge status={sub.status} />}>
          <DescriptionList
            items={[
              { label: "Plan", value: plan.name },
              { label: "Price", value: price ? `${formatMoney(price.amount_minor, price.currency)} / ${price.interval === "yearly" ? "year" : "month"}` : "Free" },
              { label: paid ? "Current period" : "This month", value: `${formatDate(sub.current_period_start)} – ${formatDate(sub.current_period_end)}` },
              { label: "Renews", value: sub.cancel_at_period_end ? "Ends at the end of this period" : paid ? formatDate(sub.current_period_end) : "—" },
            ]}
          />
          {paid && !sub.cancel_at_period_end ? (
            <div className="mt-5">
              <ConfirmButton
                action={cancelSubscriptionAction}
                confirmMessage="Stop renewing your subscription? You keep your plan until the end of the current period, then move to the free plan."
                pendingLabel="Cancelling…"
              >
                Cancel subscription
              </ConfirmButton>
            </div>
          ) : null}
        </Card>

        <Card title="Usage">
          <div className="space-y-5">
            <UsageMeter label="Call time this period" used={usage.call_seconds} limit={limits.max_call_minutes_per_month === null ? null : limits.max_call_minutes_per_month * 60} format={formatMinutes} />
            <UsageMeter label="Recording storage" used={usage.storage_bytes} limit={limits.max_storage_bytes} format={formatStorage} />
            <UsageMeter label="Agents" used={usage.agents} limit={limits.max_agents} format={plain} />
            <UsageMeter label="Departments" used={usage.departments} limit={limits.max_departments} format={plain} />
            <UsageMeter label="Widgets" used={usage.widgets} limit={limits.max_widgets} format={plain} />
          </div>
          <p className="mt-4 text-xs text-slate-500">Deleted recordings stay on file and still count toward storage.</p>
        </Card>

        <Card title="When a limit is reached">
          <PoliciesForm minutes={policies.minutes_cap_policy} storage={policies.storage_cap_policy} canOverage={paid} />
          {paid && price ? (
            <p className="mt-3 text-xs text-slate-500">
              Extra usage is billed at {formatMoney(price.overage_per_minute_minor, price.currency)} per minute and{" "}
              {formatMoney(price.overage_per_gb_minor, price.currency)} per GB of storage, charged when the period closes.
            </p>
          ) : null}
        </Card>

        <Card title="Change plan">
          <div className="grid gap-4 sm:grid-cols-2">
            {o.plans
              .filter((p) => !p.is_free && p.prices.length > 0)
              .map((p) => (
                <div key={p.id} className="rounded-lg p-4 ring-1 ring-slate-200">
                  <div className="flex items-center justify-between">
                    <h3 className="text-sm font-semibold text-slate-900">{p.name}</h3>
                    {p.id === plan.id ? <Badge tone="green">Current</Badge> : null}
                  </div>
                  {p.description ? <p className="mt-1 text-sm text-slate-600">{p.description}</p> : null}
                  <ul className="mt-3 space-y-0.5 text-sm text-slate-700">
                    <li>{p.max_agents ?? "Unlimited"} agents</li>
                    <li>{p.max_call_minutes_per_month ?? "Unlimited"} call minutes a month</li>
                    <li>{p.max_storage_bytes === null ? "Unlimited" : formatStorage(p.max_storage_bytes)} recordings</li>
                    <li>
                      {p.max_departments ?? "Unlimited"} departments · {p.max_widgets ?? "Unlimited"} widgets
                    </li>
                  </ul>
                  <div className="mt-4 space-y-2">
                    {p.prices.map((pr) => (
                      <form key={pr.id} action={checkoutAction.bind(null, pr.id)}>
                        <SubmitButton variant={pr.id === sub.plan_price_id && !sub.cancel_at_period_end ? "secondary" : "primary"} pendingLabel="Redirecting…">
                          {pr.id === sub.plan_price_id && !sub.cancel_at_period_end ? "Current: " : "Choose "}
                          {formatMoney(pr.amount_minor, pr.currency)} / {pr.interval === "yearly" ? "year" : "month"}
                        </SubmitButton>
                      </form>
                    ))}
                  </div>
                </div>
              ))}
          </div>
          {o.plans.every((p) => p.is_free || p.prices.length === 0) ? (
            <p className="text-sm text-slate-600">No paid plans are available yet.</p>
          ) : (
            <p className="mt-4 text-xs text-slate-500">
              Choosing a plan replaces your current subscription straight away. There is no proration or refund for the unused part of the old period.
            </p>
          )}
        </Card>

        <Card title="Payments">
          {o.payments.length === 0 ? (
            <EmptyState title="No payments yet" description="Payments for your subscription and extra usage appear here." />
          ) : (
            <Table>
              <thead className="bg-slate-50">
                <tr>
                  <Th>Date</Th>
                  <Th>For</Th>
                  <Th>Amount</Th>
                  <Th>Status</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {o.payments.map((p) => (
                  <tr key={p.id}>
                    <Td>{formatDateTime(p.paid_at)}</Td>
                    <Td>{p.kind === "overage" ? "Extra usage" : "Subscription"}</Td>
                    <Td>{formatMoney(p.amount_minor, p.currency)}</Td>
                    <Td>
                      <Badge tone={p.status === "succeeded" ? "green" : p.status === "failed" ? "red" : "slate"}>
                        {p.status === "succeeded" ? "Paid" : p.status === "failed" ? "Failed" : "Refunded"}
                      </Badge>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>
    </>
  );
}
