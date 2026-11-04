import type { Metadata } from "next";
import Link from "next/link";

import {
  assignCallbackAction,
  cancelCallbackAction,
  claimCallbackAction,
  completeCallbackAction,
  releaseCallbackAction,
} from "@/app/callbacks/actions";
import { AssignForm, CompleteForm } from "@/app/callbacks/CallbackForms";
import { ActionError } from "@/components/ui/ActionError";
import { CallbackStatusBadge } from "@/components/ui/Badge";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { callbacksApi } from "@/lib/api/calls";
import { departmentsApi } from "@/lib/api/departments";
import { formatDateTime } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Callback · CallCenter" };

export default async function CallbackPage({ params, searchParams }: PageProps<"/callbacks/[id]">) {
  const user = await requireRole("agent", "supervisor", "tenant_admin");
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  const cb = await orNotFound(callbacksApi.get(id));
  const isAdmin = user.role === "tenant_admin";
  const mine = cb.claimed_by === user.id;
  const members = isAdmin && (cb.status === "open" || cb.status === "claimed") ? (await departmentsApi.get(cb.department_id)).members : [];

  return (
    <>
      <PageHeader
        title={`Callback for ${cb.name}`}
        description={`${cb.department_name} · requested ${formatDateTime(cb.created_at)}`}
        back={{ href: "/callbacks", label: "Callbacks" }}
        actions={<CallbackStatusBadge status={cb.status} />}
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />

      <div className="space-y-6">
        <Card title="Who to call">
          <DescriptionList
            items={[
              { label: "Name", value: cb.name },
              { label: "Phone", value: cb.phone ?? "—" },
              { label: "Email", value: cb.email ?? "—" },
              { label: "Reason", value: cb.reason || "—" },
              { label: "Claimed by", value: cb.claimed_by_name ?? "—" },
              { label: "Claimed", value: formatDateTime(cb.claimed_at) },
            ]}
          />
          {cb.call_id && isAdmin ? (
            <p className="mt-4 text-sm">
              <Link href={`/calls/${cb.call_id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                View the call this came from
              </Link>
            </p>
          ) : null}
        </Card>

        {cb.status === "completed" ? (
          <Card title="Outcome">
            <p className="whitespace-pre-wrap text-sm text-slate-700">{cb.resolution_notes || "Completed without notes."}</p>
            <p className="mt-2 text-xs text-slate-500">Completed {formatDateTime(cb.resolved_at)}.</p>
          </Card>
        ) : null}

        {cb.status === "open" ? (
          <Card title="Take this callback">
            <p className="mb-4 text-sm text-slate-600">Claim it so nobody else calls the same person.</p>
            <form action={claimCallbackAction.bind(null, cb.id)}>
              <SubmitButton pendingLabel="Claiming…">Claim callback</SubmitButton>
            </form>
          </Card>
        ) : null}

        {cb.status === "claimed" && (mine || isAdmin) ? (
          <Card title="After you have called">
            <div className="space-y-5">
              <CompleteForm action={completeCallbackAction.bind(null, cb.id)} />
              <ConfirmButton
                action={releaseCallbackAction.bind(null, cb.id)}
                confirmMessage="Release this callback so another agent can take it?"
                pendingLabel="Releasing…"
              >
                Release to the queue
              </ConfirmButton>
            </div>
          </Card>
        ) : null}

        {isAdmin && (cb.status === "open" || cb.status === "claimed") ? (
          <Card title="Administration">
            <div className="space-y-5">
              <AssignForm action={assignCallbackAction.bind(null, cb.id)} members={members} />
              <ConfirmButton
                action={cancelCallbackAction.bind(null, cb.id)}
                confirmMessage="Cancel this callback request? Nobody will call this person back."
                variant="danger"
                pendingLabel="Cancelling…"
              >
                Cancel request
              </ConfirmButton>
            </div>
          </Card>
        ) : null}
      </div>
    </>
  );
}
