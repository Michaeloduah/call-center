import type { Metadata } from "next";

import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { Table, Td, Th } from "@/components/ui/Table";
import { buttonClass } from "@/components/ui/Button";
import { inputClass } from "@/components/ui/Field";
import { auditApi } from "@/lib/api/settings";
import { formatDateTime, pageParam, param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Audit log · CallCenter" };

export default async function AuditLogsPage({ searchParams }: PageProps<"/platform/audit-logs">) {
  await requireRole("platform_admin");
  const sp = await searchParams;
  const filters = { action: param(sp.action), tenant_id: param(sp.tenant_id) };
  const data = await auditApi.list({ ...filters, page: pageParam(sp.page) });

  return (
    <>
      <PageHeader title="Audit log" description="Security-relevant actions across all tenants, newest first." />
      <form action="/platform/audit-logs" method="get" className="mb-4 flex flex-wrap items-end gap-3">
        <div className="min-w-56 flex-1">
          <label htmlFor="action" className="sr-only">
            Action
          </label>
          <input id="action" name="action" defaultValue={filters.action} placeholder="Exact action, e.g. tenant.suspended" className={inputClass} />
        </div>
        {filters.tenant_id ? <input type="hidden" name="tenant_id" value={filters.tenant_id} /> : null}
        <button type="submit" className={buttonClass("secondary")}>
          Filter
        </button>
      </form>
      {data.items.length === 0 ? (
        <EmptyState title="No audit entries" description={filters.action ? "No entries match that action." : undefined} />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>When</Th>
                <Th>Action</Th>
                <Th>Tenant</Th>
                <Th>Actor</Th>
                <Th>IP</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((e) => (
                <tr key={e.id}>
                  <Td>{formatDateTime(e.created_at)}</Td>
                  <Td className="font-mono text-xs">{e.action}</Td>
                  <Td>{e.tenant_name ?? <span className="text-slate-400">Platform</span>}</Td>
                  <Td>{e.actor_email ?? <span className="text-slate-400">System</span>}</Td>
                  <Td className="font-mono text-xs">{e.ip ?? "—"}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/platform/audit-logs" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
