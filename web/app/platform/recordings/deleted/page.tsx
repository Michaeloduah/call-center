import type { Metadata } from "next";

import { platformRecordingAccessAction, restoreRecordingAction } from "@/app/calls/actions";
import { RecordingPlayer } from "@/components/calls/RecordingPlayer";
import { ActionError } from "@/components/ui/ActionError";
import { Card } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { EmptyState } from "@/components/ui/EmptyState";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { Pagination } from "@/components/ui/Pagination";
import { platformRecordingsApi } from "@/lib/api/calls";
import { formatBytes, formatDateTime, formatDuration, pageParam } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Deleted recordings · CallCenter" };

export default async function DeletedRecordingsPage({ searchParams }: PageProps<"/platform/recordings/deleted">) {
  await requireRole("platform_admin");
  const sp = await searchParams;
  const data = await platformRecordingsApi.listDeleted({ page: pageParam(sp.page) });

  return (
    <>
      <PageHeader
        title="Deleted recordings"
        description="Recordings that tenant administrators deleted. The files are kept and still count toward each tenant's storage."
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />
      {data.items.length === 0 ? (
        <EmptyState title="Nothing has been deleted" description="Recordings deleted by tenants appear here." />
      ) : (
        <>
          <div className="space-y-4">
            {data.items.map((r) => (
              <Card key={r.id} title={r.tenant_name}>
                <p className="text-sm text-slate-700">
                  {r.department_name} · {r.caller_name ?? "Anonymous caller"} ·{" "}
                  {r.duration_seconds !== null ? formatDuration(r.duration_seconds) : "—"} · {formatBytes(r.size_bytes)}
                </p>
                <p className="mt-1 text-xs text-slate-500">
                  Deleted {formatDateTime(r.deleted_at)}
                  {r.deleted_by_email ? ` by ${r.deleted_by_email}` : ""}
                </p>
                <div className="mt-4 space-y-3">
                  <RecordingPlayer access={platformRecordingAccessAction.bind(null, r.id)} />
                  <ConfirmButton
                    action={restoreRecordingAction.bind(null, r.id)}
                    confirmMessage="Restore this recording? Its tenant will be able to see it again."
                    pendingLabel="Restoring…"
                  >
                    Restore
                  </ConfirmButton>
                </div>
              </Card>
            ))}
          </div>
          <Pagination basePath="/platform/recordings/deleted" page={data.page} perPage={data.per_page} total={data.total} query={{}} />
        </>
      )}
    </>
  );
}
