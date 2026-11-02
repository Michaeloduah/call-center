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
import { departmentsApi } from "@/lib/api/departments";
import { pageParam, param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Departments · CallCenter" };

export default async function DepartmentsPage({ searchParams }: PageProps<"/admin/departments">) {
  await requireRole("tenant_admin");
  const sp = await searchParams;
  const filters = { q: param(sp.q), active: param(sp.active) };
  const data = await departmentsApi.list({ ...filters, page: pageParam(sp.page) });
  const filtered = filters.q !== "" || filters.active !== "";

  return (
    <>
      <PageHeader
        title="Departments"
        description="Calls from each widget are routed to one department's agents."
        actions={<LinkButton href="/admin/departments/new">New department</LinkButton>}
      />
      <Notice value={sp.notice} />
      <FilterBar
        action="/admin/departments"
        search={{ value: filters.q, placeholder: "Search by name" }}
        selects={[
          {
            name: "active",
            label: "Status",
            value: filters.active,
            options: [
              { value: "", label: "All" },
              { value: "true", label: "Active" },
              { value: "false", label: "Inactive" },
            ],
          },
        ]}
      />
      {data.items.length === 0 ? (
        <EmptyState
          title={filtered ? "No departments match your filters" : "No departments yet"}
          description={filtered ? "Try a different search." : "Create departments such as Sales or Support."}
          action={filtered ? undefined : <LinkButton href="/admin/departments/new">New department</LinkButton>}
        />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>Name</Th>
                <Th>Status</Th>
                <Th>Agents</Th>
                <Th>Supervisors</Th>
                <Th>Timezone</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((d) => (
                <tr key={d.id} className="hover:bg-slate-50">
                  <Td>
                    <Link href={`/admin/departments/${d.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                      {d.name}
                    </Link>
                  </Td>
                  <Td>{d.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge>}</Td>
                  <Td>{d.members_count}</Td>
                  <Td>{d.supervisors_count}</Td>
                  <Td>{d.timezone}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/admin/departments" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
