import type { Metadata } from "next";
import Link from "next/link";

import { CallbackStatusBadge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { FilterBar } from "@/components/ui/FilterBar";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { Table, Td, Th } from "@/components/ui/Table";
import { callbacksApi } from "@/lib/api/calls";
import { callbackStatusLabels, formatDateTime, pageParam, param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";
import type { CallbackStatus } from "@/lib/types";

export const metadata: Metadata = { title: "Callbacks · CallCenter" };

const statuses = Object.keys(callbackStatusLabels) as CallbackStatus[];

export default async function CallbacksPage({ searchParams }: PageProps<"/callbacks">) {
  const user = await requireRole("agent", "supervisor", "tenant_admin");
  const sp = await searchParams;
  const filters = { status: param(sp.status) };
  const data = await callbacksApi.list({ ...filters, page: pageParam(sp.page) });

  return (
    <>
      <PageHeader
        title="Callbacks"
        description={
          user.role === "tenant_admin"
            ? "People who asked to be called back. Open requests are listed first."
            : "Callback requests for your departments. Claim one to call the person back."
        }
      />
      <Notice value={sp.notice} />
      <FilterBar
        action="/callbacks"
        selects={[
          {
            name: "status",
            label: "Status",
            value: filters.status,
            options: [{ value: "", label: "All statuses" }, ...statuses.map((s) => ({ value: s, label: callbackStatusLabels[s] }))],
          },
        ]}
      />
      {data.items.length === 0 ? (
        <EmptyState
          title={filters.status ? "No callbacks with that status" : "No callback requests"}
          description={filters.status ? undefined : "When a caller cannot reach anyone they can leave their details here."}
        />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>Requested</Th>
                <Th>Name</Th>
                <Th>Contact</Th>
                <Th>Department</Th>
                <Th>Status</Th>
                <Th>Claimed by</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((c) => (
                <tr key={c.id} className="hover:bg-slate-50">
                  <Td>
                    <Link href={`/callbacks/${c.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                      {formatDateTime(c.created_at)}
                    </Link>
                  </Td>
                  <Td>{c.name}</Td>
                  <Td>{c.phone ?? c.email ?? "—"}</Td>
                  <Td>{c.department_name}</Td>
                  <Td>
                    <CallbackStatusBadge status={c.status} />
                  </Td>
                  <Td>{c.claimed_by_name ?? "—"}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/callbacks" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
