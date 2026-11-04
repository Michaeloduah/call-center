import type { Metadata } from "next";
import Link from "next/link";

import { buttonClass } from "@/components/ui/Button";
import { CallStatusBadge, RecordingStatusBadge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { inputClass } from "@/components/ui/Field";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { Table, Td, Th } from "@/components/ui/Table";
import { callsApi } from "@/lib/api/calls";
import { departmentsApi } from "@/lib/api/departments";
import { callStatusLabels, formatDateTime, formatDuration, pageParam, param } from "@/lib/format";
import { requireRole } from "@/lib/server/session";
import type { CallStatus } from "@/lib/types";

export const metadata: Metadata = { title: "Call history · CallCenter" };

const statuses = Object.keys(callStatusLabels) as CallStatus[];

export default async function CallsPage({ searchParams }: PageProps<"/calls">) {
  const user = await requireRole("agent", "supervisor", "tenant_admin");
  const sp = await searchParams;
  const filters = {
    q: param(sp.q),
    status: param(sp.status),
    department_id: param(sp.department_id),
    from: param(sp.from),
    to: param(sp.to),
  };
  // Only administrators may list departments; everyone else already sees just their own calls.
  const departments = user.role === "tenant_admin" ? await departmentsApi.listAll() : [];
  const data = await callsApi.list({ ...filters, page: pageParam(sp.page) });
  const filtered = Object.values(filters).some(Boolean);

  return (
    <>
      <PageHeader
        title="Call history"
        description={
          user.role === "tenant_admin"
            ? "Every call to your organisation."
            : user.role === "supervisor"
              ? "Calls to the departments you supervise and calls you handled."
              : "Calls you handled."
        }
      />
      <form action="/calls" method="get" role="search" className="mb-4 flex flex-wrap items-end gap-3">
        <div className="min-w-56 flex-1">
          <label htmlFor="q" className="sr-only">
            Search
          </label>
          <input id="q" name="q" type="search" defaultValue={filters.q} placeholder="Search by caller name, phone or email" className={inputClass} />
        </div>
        <div>
          <label htmlFor="status" className="sr-only">
            Status
          </label>
          <select id="status" name="status" defaultValue={filters.status} className={`${inputClass} bg-white`}>
            <option value="">All statuses</option>
            {statuses.map((s) => (
              <option key={s} value={s}>
                {callStatusLabels[s]}
              </option>
            ))}
          </select>
        </div>
        {departments.length > 0 ? (
          <div>
            <label htmlFor="department_id" className="sr-only">
              Department
            </label>
            <select id="department_id" name="department_id" defaultValue={filters.department_id} className={`${inputClass} bg-white`}>
              <option value="">All departments</option>
              {departments.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
        ) : null}
        <div>
          <label htmlFor="from" className="block text-xs text-slate-500">
            From (UTC)
          </label>
          <input id="from" name="from" type="date" defaultValue={filters.from} className={inputClass} />
        </div>
        <div>
          <label htmlFor="to" className="block text-xs text-slate-500">
            To (UTC)
          </label>
          <input id="to" name="to" type="date" defaultValue={filters.to} className={inputClass} />
        </div>
        <button type="submit" className={buttonClass("secondary")}>
          Filter
        </button>
      </form>

      {data.items.length === 0 ? (
        <EmptyState
          title={filtered ? "No calls match your filters" : "No calls yet"}
          description={filtered ? undefined : "Calls appear here once callers start using your widgets."}
        />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>Started</Th>
                <Th>Caller</Th>
                <Th>Department</Th>
                <Th>Agent</Th>
                <Th>Status</Th>
                <Th>Duration</Th>
                <Th>Recording</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((c) => (
                <tr key={c.id} className="hover:bg-slate-50">
                  <Td>
                    <Link href={`/calls/${c.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                      {formatDateTime(c.queued_at)}
                    </Link>
                  </Td>
                  <Td>
                    {c.caller_name ?? "Anonymous"}
                    {c.caller_phone ? <span className="block text-xs text-slate-500">{c.caller_phone}</span> : null}
                  </Td>
                  <Td>{c.department_name}</Td>
                  <Td>{c.agent_name ?? "—"}</Td>
                  <Td>
                    <CallStatusBadge status={c.status} />
                  </Td>
                  <Td>{c.duration_seconds !== null ? formatDuration(c.duration_seconds) : "—"}</Td>
                  <Td>{c.recording_status ? <RecordingStatusBadge status={c.recording_status} /> : "—"}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/calls" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
