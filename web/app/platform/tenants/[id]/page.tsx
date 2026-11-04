import type { Metadata } from "next";

import {
  activateTenantAction,
  deleteTenantAction,
  suspendTenantAction,
} from "@/app/platform/tenants/actions";
import { ActionError } from "@/components/ui/ActionError";
import { SubscriptionStatusBadge, TenantStatusBadge, UserStatusBadge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { plansApi } from "@/lib/api/billing";
import { tenantsApi } from "@/lib/api/tenants";
import { formatDate, formatDateTime, formatMinutes, formatMoney, formatStorage } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Tenant · CallCenter" };

export default async function TenantDetailPage({ params, searchParams }: PageProps<"/platform/tenants/[id]">) {
  await requireRole("platform_admin");
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  const [tenant, billing] = await Promise.all([orNotFound(tenantsApi.get(id)), orNotFound(plansApi.tenantBilling(id))]);
  const suspended = tenant.status === "suspended";

  return (
    <>
      <PageHeader
        title={tenant.name}
        back={{ href: "/platform/tenants", label: "Tenants" }}
        actions={
          <>
            <LinkButton href={`/platform/tenants/${tenant.id}/edit`} variant="secondary">
              Edit
            </LinkButton>
            {suspended || tenant.status === "pending_verification" ? (
              <ConfirmButton
                action={activateTenantAction.bind(null, tenant.id)}
                confirmMessage={`Activate ${tenant.name}?`}
                pendingLabel="Activating…"
              >
                Activate
              </ConfirmButton>
            ) : null}
            {!suspended ? (
              <ConfirmButton
                action={suspendTenantAction.bind(null, tenant.id)}
                confirmMessage={`Suspend ${tenant.name}? All of its users will be signed out and blocked from signing in.`}
                pendingLabel="Suspending…"
              >
                Suspend
              </ConfirmButton>
            ) : (
              <ConfirmButton
                action={deleteTenantAction.bind(null, tenant.id)}
                confirmMessage={`Permanently delete ${tenant.name} and ALL of its data? This cannot be undone.`}
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
        <Card title="Details">
          <DescriptionList
            items={[
              { label: "Status", value: <TenantStatusBadge status={tenant.status} /> },
              { label: "Slug", value: <span className="font-mono text-xs">{tenant.slug}</span> },
              { label: "Users", value: tenant.users_count },
              { label: "Departments", value: tenant.departments_count },
              { label: "Created", value: formatDateTime(tenant.created_at) },
              { label: "Last updated", value: formatDateTime(tenant.updated_at) },
            ]}
          />
          {suspended ? (
            <p className="mt-4 text-sm text-slate-500">This tenant is suspended. Deleting it permanently removes all of its data.</p>
          ) : null}
        </Card>
        <Card title="Subscription" actions={<SubscriptionStatusBadge status={billing.subscription.status} />}>
          <DescriptionList
            items={[
              { label: "Plan", value: billing.plan.name },
              {
                label: "Price",
                value: billing.price ? `${formatMoney(billing.price.amount_minor, billing.price.currency)} / ${billing.price.interval === "yearly" ? "year" : "month"}` : "Free",
              },
              { label: "Period", value: `${formatDate(billing.subscription.current_period_start)} – ${formatDate(billing.subscription.current_period_end)}` },
              { label: "Provider", value: billing.subscription.provider ?? "—" },
              { label: "Call time this period", value: formatMinutes(billing.usage.call_seconds) },
              { label: "Recording storage", value: formatStorage(billing.usage.storage_bytes) },
            ]}
          />
          {billing.payments.length > 0 ? (
            <ul className="mt-4 divide-y divide-slate-100 text-sm">
              {billing.payments.slice(0, 5).map((pay) => (
                <li key={pay.id} className="flex justify-between py-1.5">
                  <span className="text-slate-600">
                    {formatDateTime(pay.paid_at)} · {pay.kind === "overage" ? "extra usage" : "subscription"}
                  </span>
                  <span className={pay.status === "failed" ? "text-red-600" : "text-slate-900"}>
                    {formatMoney(pay.amount_minor, pay.currency)} {pay.status}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </Card>
        <Card title="Admins">
          {tenant.admins.length === 0 ? (
            <p className="text-sm text-slate-500">This tenant has no admins.</p>
          ) : (
            <ul className="divide-y divide-slate-100">
              {tenant.admins.map((a) => (
                <li key={a.id} className="flex items-center justify-between py-2 text-sm">
                  <span>
                    <span className="font-medium text-slate-900">{a.full_name}</span>
                    <span className="ml-2 text-slate-500">{a.email}</span>
                  </span>
                  <UserStatusBadge status={a.status} />
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </>
  );
}
