import type { Metadata } from "next";
import Link from "next/link";

import { Badge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FilterBar } from "@/components/ui/FilterBar";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { Table, Td, Th } from "@/components/ui/Table";
import { widgetsApi } from "@/lib/api/widgets";
import { formatDate, pageParam, param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Widgets · CallCenter" };

export default async function WidgetsPage({ searchParams }: PageProps<"/admin/widgets">) {
  await requireRole("tenant_admin");
  const sp = await searchParams;
  const filters = { q: param(sp.q) };
  const data = await widgetsApi.list({ ...filters, page: pageParam(sp.page) });

  return (
    <>
      <PageHeader
        title="Widgets"
        description="“Call us” buttons for your websites. Each widget routes calls to one department."
        actions={<LinkButton href="/admin/widgets/new">New widget</LinkButton>}
      />
      <Notice value={sp.notice} />
      <FilterBar action="/admin/widgets" search={{ value: filters.q, placeholder: "Search by widget or department" }} />
      {data.items.length === 0 ? (
        <EmptyState
          title={filters.q ? "No widgets match your search" : "No widgets yet"}
          description={filters.q ? undefined : "Create a widget, then paste its embed code into your website."}
          action={filters.q ? undefined : <LinkButton href="/admin/widgets/new">New widget</LinkButton>}
        />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>Name</Th>
                <Th>Department</Th>
                <Th>Status</Th>
                <Th>Websites</Th>
                <Th>Created</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((w) => (
                <tr key={w.id} className="hover:bg-slate-50">
                  <Td>
                    <Link href={`/admin/widgets/${w.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                      {w.name}
                    </Link>
                  </Td>
                  <Td>{w.department_name}</Td>
                  <Td>{w.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge>}</Td>
                  <Td>{w.allowed_origins.length}</Td>
                  <Td>{formatDate(w.created_at)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/admin/widgets" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
