import type { Metadata } from "next";
import Link from "next/link";

import {
  deleteUserAction,
  disableUserAction,
  enableUserAction,
  resendInviteAction,
} from "@/app/admin/users/actions";
import { ActionError } from "@/components/ui/ActionError";
import { UserStatusBadge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { usersApi } from "@/lib/api/users";
import { formatDateTime, roleLabels } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";
import type { DepartmentRef } from "@/lib/types";

export const metadata: Metadata = { title: "User · CallCenter" };

function DepartmentLinks({ items, empty }: { items: DepartmentRef[]; empty: string }) {
  if (items.length === 0) return <p className="text-sm text-slate-500">{empty}</p>;
  return (
    <ul className="flex flex-wrap gap-2">
      {items.map((d) => (
        <li key={d.id}>
          <Link
            href={`/admin/departments/${d.id}`}
            className="inline-flex rounded-md bg-slate-100 px-2.5 py-1 text-sm text-slate-800 hover:bg-slate-200"
          >
            {d.name}
          </Link>
        </li>
      ))}
    </ul>
  );
}

export default async function UserDetailPage({ params, searchParams }: PageProps<"/admin/users/[id]">) {
  const [{ id }, sp, me] = await Promise.all([params, searchParams, requireRole("tenant_admin")]);
  const user = await orNotFound(usersApi.get(id));
  const isSelf = me.id === user.id;

  return (
    <>
      <PageHeader
        title={user.full_name}
        description={user.email}
        back={{ href: "/admin/users", label: "Users" }}
        actions={
          <>
            <LinkButton href={`/admin/users/${user.id}/edit`} variant="secondary">
              Edit
            </LinkButton>
            {user.status === "invited" ? (
              <ConfirmButton
                action={resendInviteAction.bind(null, user.id)}
                confirmMessage={`Send a new invitation to ${user.email}? The previous link will stop working.`}
                pendingLabel="Sending…"
              >
                Resend invitation
              </ConfirmButton>
            ) : null}
            {!isSelf && user.status !== "disabled" ? (
              <ConfirmButton
                action={disableUserAction.bind(null, user.id)}
                confirmMessage={`Disable ${user.full_name}? They will be signed out and unable to sign in.`}
                pendingLabel="Disabling…"
              >
                Disable
              </ConfirmButton>
            ) : null}
            {!isSelf && user.status === "disabled" ? (
              <ConfirmButton action={enableUserAction.bind(null, user.id)} confirmMessage={`Enable ${user.full_name}?`} pendingLabel="Enabling…">
                Enable
              </ConfirmButton>
            ) : null}
            {!isSelf ? (
              <ConfirmButton
                action={deleteUserAction.bind(null, user.id)}
                confirmMessage={`Permanently delete ${user.full_name}? This cannot be undone.`}
                variant="danger"
                pendingLabel="Deleting…"
              >
                Delete
              </ConfirmButton>
            ) : null}
          </>
        }
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />
      <div className="space-y-6">
        <Card title="Details">
          <DescriptionList
            items={[
              { label: "Role", value: roleLabels[user.role] },
              { label: "Status", value: <UserStatusBadge status={user.status} /> },
              { label: "Email verified", value: formatDateTime(user.email_verified_at) },
              { label: "Last sign-in", value: formatDateTime(user.last_login_at) },
              { label: "Created", value: formatDateTime(user.created_at) },
            ]}
          />
        </Card>
        <Card title="Takes calls for">
          <DepartmentLinks items={user.departments} empty="Not assigned to any department." />
        </Card>
        {user.role === "supervisor" ? (
          <Card title="Supervises">
            <DepartmentLinks items={user.supervised_departments} empty="Not supervising any department." />
          </Card>
        ) : null}
      </div>
    </>
  );
}
