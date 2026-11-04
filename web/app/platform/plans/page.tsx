import type { Metadata } from "next";
import Link from "next/link";

import { Badge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Table, Td, Th } from "@/components/ui/Table";
import { plansApi } from "@/lib/api/billing";
import { formatMoney, formatStorage } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Plans · CallCenter" };

const unlimited = (n: number | null) => (n === null ? "∞" : String(n));

export default async function PlansPage({ searchParams }: PageProps<"/platform/plans">) {
  await requireRole("platform_admin");
  const sp = await searchParams;
  const plans = await plansApi.list();

  return (
    <>
      <PageHeader
        title="Plans"
        description="What tenants can subscribe to. Every tenant starts on the free plan."
        actions={<LinkButton href="/platform/plans/new">New plan</LinkButton>}
      />
      <Notice value={sp.notice} />
      {plans.length === 0 ? (
        <EmptyState title="No plans" />
      ) : (
        <Table>
          <thead className="bg-slate-50">
            <tr>
              <Th>Plan</Th>
              <Th>Limits</Th>
              <Th>Prices</Th>
              <Th>Status</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {plans.map((p) => (
              <tr key={p.id} className="hover:bg-slate-50">
                <Td>
                  <Link href={`/platform/plans/${p.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                    {p.name}
                  </Link>
                  <span className="ml-2 text-xs text-slate-500">{p.code}</span>
                </Td>
                <Td>
                  {unlimited(p.max_agents)} agents · {unlimited(p.max_call_minutes_per_month)} min ·{" "}
                  {p.max_storage_bytes === null ? "∞" : formatStorage(p.max_storage_bytes)} · {unlimited(p.max_departments)} depts ·{" "}
                  {unlimited(p.max_widgets)} widgets
                </Td>
                <Td>{p.is_free ? "Free" : p.prices.length === 0 ? "None yet" : p.prices.map((x) => `${formatMoney(x.amount_minor, x.currency)}/${x.interval === "yearly" ? "yr" : "mo"}`).join(", ")}</Td>
                <Td>
                  {p.is_free ? <Badge tone="indigo">Free tier</Badge> : p.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge>}
                  {!p.is_free && !p.is_public ? <Badge>Hidden</Badge> : null}
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </>
  );
}
