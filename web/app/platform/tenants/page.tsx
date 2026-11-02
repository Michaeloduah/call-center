import type { Metadata } from "next";
import Link from "next/link";

import { TenantStatusBadge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FilterBar } from "@/components/ui/FilterBar";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { Table, Td, Th } from "@/components/ui/Table";
import { tenantsApi } from "@/lib/api/tenants";
import { formatDate, pageParam, param, tenantStatusLabels } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Tenants · CallCenter" };

export default async function TenantsPage({ searchParams }: PageProps<"/platform/tenants">) {
  await requireRole("platform_admin");
  const sp = await searchParams;
  const filters = { q: param(sp.q), status: param(sp.status) };
  const data = await tenantsApi.list({ ...filters, page: pageParam(sp.page) });
  const filtered = filters.q !== "" || filters.status !== "";

  return (
    <>
      <PageHeader
        title="Tenants"
        description="Businesses using the platform."
        actions={<LinkButton href="/platform/tenants/new">New tenant</LinkButton>}
      />
      <Notice value={sp.notice} />
      <FilterBar
        action="/platform/tenants"
        search={{ value: filters.q, placeholder: "Search by name or slug" }}
        selects={[
          {
            name: "status",
            label: "Status",
            value: filters.status,
            options: [{ value: "", label: "All statuses" }, ...Object.entries(tenantStatusLabels).map(([value, label]) => ({ value, label }))],
          },
        ]}
      />
      {data.items.length === 0 ? (
        <EmptyState
          title={filtered ? "No tenants match your filters" : "No tenants yet"}
          description={filtered ? "Try a different search or status." : "Create a tenant or wait for businesses to sign up."}
          action={filtered ? undefined : <LinkButton href="/platform/tenants/new">New tenant</LinkButton>}
        />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>Name</Th>
                <Th>Slug</Th>
                <Th>Status</Th>
                <Th>Users</Th>
                <Th>Created</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((t) => (
                <tr key={t.id} className="hover:bg-slate-50">
                  <Td>
                    <Link href={`/platform/tenants/${t.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                      {t.name}
                    </Link>
                  </Td>
                  <Td className="font-mono text-xs">{t.slug}</Td>
                  <Td>
                    <TenantStatusBadge status={t.status} />
                  </Td>
                  <Td>{t.users_count}</Td>
                  <Td>{formatDate(t.created_at)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/platform/tenants" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
