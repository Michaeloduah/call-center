import type { Metadata } from "next";

import { deletePlanAction, deletePriceAction, setPriceActiveAction, syncPriceAction } from "@/app/platform/plans/actions";
import { AddPriceForm } from "@/app/platform/plans/PlanForms";
import { ActionError } from "@/components/ui/ActionError";
import { Badge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Table, Td, Th } from "@/components/ui/Table";
import { plansApi } from "@/lib/api/billing";
import { formatMoney, formatStorage } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Plan · CallCenter" };

const unlimited = (n: number | null) => (n === null ? "Unlimited" : String(n));

export default async function PlanPage({ params, searchParams }: PageProps<"/platform/plans/[id]">) {
  await requireRole("platform_admin");
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  const plan = await orNotFound(plansApi.get(id));

  return (
    <>
      <PageHeader
        title={plan.name}
        description={plan.description || undefined}
        back={{ href: "/platform/plans", label: "Plans" }}
        actions={
          <>
            <LinkButton href={`/platform/plans/${plan.id}/edit`} variant="secondary">
              Edit
            </LinkButton>
            {plan.is_free ? null : (
              <ConfirmButton
                action={deletePlanAction.bind(null, plan.id)}
                confirmMessage="Delete this plan and its prices? Plans with subscribers cannot be deleted."
                variant="danger"
                pendingLabel="Deleting…"
              >
                Delete
              </ConfirmButton>
            )}
          </>
        }
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />

      <div className="space-y-6">
        <Card title="Limits" actions={plan.is_free ? <Badge tone="indigo">Free tier</Badge> : plan.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge>}>
          <DescriptionList
            items={[
              { label: "Agent seats", value: unlimited(plan.max_agents) },
              { label: "Call minutes per month", value: unlimited(plan.max_call_minutes_per_month) },
              { label: "Recording storage", value: plan.max_storage_bytes === null ? "Unlimited" : formatStorage(plan.max_storage_bytes) },
              { label: "Departments", value: unlimited(plan.max_departments) },
              { label: "Widgets", value: unlimited(plan.max_widgets) },
              { label: "Visible to tenants", value: plan.is_public ? "Yes" : "No" },
            ]}
          />
        </Card>

        {plan.is_free ? (
          <Card title="Prices">
            <p className="text-sm text-slate-600">The free plan has no prices.</p>
          </Card>
        ) : (
          <>
            <Card title="Prices">
              {plan.prices.length === 0 ? (
                <p className="text-sm text-slate-600">No prices yet. Add one below so tenants can subscribe.</p>
              ) : (
                <Table>
                  <thead className="bg-slate-50">
                    <tr>
                      <Th>Price</Th>
                      <Th>Extra usage</Th>
                      <Th>Provider</Th>
                      <Th>Status</Th>
                      <Th />
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {plan.prices.map((p) => (
                      <tr key={p.id}>
                        <Td>
                          {formatMoney(p.amount_minor, p.currency)} / {p.interval === "yearly" ? "year" : "month"}
                        </Td>
                        <Td>
                          {formatMoney(p.overage_per_minute_minor, p.currency)} / min · {formatMoney(p.overage_per_gb_minor, p.currency)} / GB
                        </Td>
                        <Td>
                          {p.provider} {p.synced || p.provider === "fake" ? <Badge tone="green">Ready</Badge> : <Badge tone="amber">Not synced</Badge>}
                        </Td>
                        <Td>{p.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge>}</Td>
                        <Td>
                          <div className="flex flex-wrap gap-2">
                            {p.provider === "fake" || p.synced ? null : (
                              <ConfirmButton action={syncPriceAction.bind(null, plan.id, p.id)} confirmMessage={`Create this price at ${p.provider}?`} pendingLabel="Syncing…">
                                Sync
                              </ConfirmButton>
                            )}
                            <ConfirmButton
                              action={setPriceActiveAction.bind(null, plan.id, p.id, !p.is_active, p.overage_per_minute_minor, p.overage_per_gb_minor)}
                              confirmMessage={p.is_active ? "Hide this price from tenants? Existing subscribers keep it." : "Offer this price to tenants again?"}
                              pendingLabel="Saving…"
                            >
                              {p.is_active ? "Deactivate" : "Activate"}
                            </ConfirmButton>
                            <ConfirmButton
                              action={deletePriceAction.bind(null, plan.id, p.id)}
                              confirmMessage="Delete this price? Prices with subscribers cannot be deleted."
                              variant="danger"
                              pendingLabel="Deleting…"
                            >
                              Delete
                            </ConfirmButton>
                          </div>
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              )}
              <p className="mt-3 text-xs text-slate-500">
                Prices never change in place: to change an amount, add a new price and deactivate the old one. Existing subscribers keep the price they signed up at.
              </p>
            </Card>
            <Card title="Add a price">
              <AddPriceForm planId={plan.id} />
            </Card>
          </>
        )}
      </div>
    </>
  );
}
