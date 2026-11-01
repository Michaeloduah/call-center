import type { Metadata } from "next";
import Link from "next/link";

import { UserStatusBadge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FilterBar } from "@/components/ui/FilterBar";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { Table, Td, Th } from "@/components/ui/Table";
import { usersApi } from "@/lib/api/users";
import { formatDateTime, pageParam, param, roleLabels, userStatusLabels } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Users · CallCenter" };

export default async function UsersPage({ searchParams }: PageProps<"/admin/users">) {
  await requireRole("tenant_admin");
  const sp = await searchParams;
  const filters = { q: param(sp.q), role: param(sp.role), status: param(sp.status) };
  const data = await usersApi.list({ ...filters, page: pageParam(sp.page) });
  const filtered = Object.values(filters).some((v) => v !== "");

  return (
    <>
      <PageHeader
        title="Users"
        description="Admins, supervisors and agents in your organisation."
        actions={<LinkButton href="/admin/users/new">Invite user</LinkButton>}
      />
      <Notice value={sp.notice} />
      <FilterBar
        action="/admin/users"
        search={{ value: filters.q, placeholder: "Search by name or email" }}
        selects={[
          {
            name: "role",
            label: "Role",
            value: filters.role,
            options: [
              { value: "", label: "All roles" },
              { value: "tenant_admin", label: roleLabels.tenant_admin },
              { value: "supervisor", label: roleLabels.supervisor },
              { value: "agent", label: roleLabels.agent },
            ],
          },
          {
            name: "status",
            label: "Status",
            value: filters.status,
            options: [{ value: "", label: "All statuses" }, ...Object.entries(userStatusLabels).map(([value, label]) => ({ value, label }))],
          },
        ]}
      />
      {data.items.length === 0 ? (
        <EmptyState
          title={filtered ? "No users match your filters" : "No users yet"}
          description={filtered ? "Try a different search, role or status." : "Invite your first agent to get started."}
          action={filtered ? undefined : <LinkButton href="/admin/users/new">Invite user</LinkButton>}
        />
      ) : (
        <>
          <Table>
            <thead className="bg-slate-50">
              <tr>
                <Th>Name</Th>
                <Th>Role</Th>
                <Th>Status</Th>
                <Th>Last sign-in</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.items.map((u) => (
                <tr key={u.id} className="hover:bg-slate-50">
                  <Td>
                    <Link href={`/admin/users/${u.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                      {u.full_name}
                    </Link>
                    <p className="text-xs text-slate-500">{u.email}</p>
                  </Td>
                  <Td>{roleLabels[u.role]}</Td>
                  <Td>
                    <UserStatusBadge status={u.status} />
                  </Td>
                  <Td>{formatDateTime(u.last_login_at)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Pagination basePath="/admin/users" page={data.page} perPage={data.per_page} total={data.total} query={filters} />
        </>
      )}
    </>
  );
}
