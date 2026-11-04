import type { Metadata } from "next";

import { deleteRecordingAction, recordingAccessAction } from "@/app/calls/actions";
import { RecordingPlayer } from "@/components/calls/RecordingPlayer";
import { ActionError } from "@/components/ui/ActionError";
import { CallStatusBadge, CallbackStatusBadge, RecordingStatusBadge } from "@/components/ui/Badge";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { callsApi } from "@/lib/api/calls";
import { endReasonLabels, formatBytes, formatDateTime, formatDuration } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";
import Link from "next/link";

export const metadata: Metadata = { title: "Call · CallCenter" };

const outcomeLabels: Record<string, string> = {
  pending: "Ringing",
  accepted: "Answered",
  declined: "Declined",
  timeout: "No answer",
  cancelled: "Cancelled",
};

export default async function CallPage({ params, searchParams }: PageProps<"/calls/[id]">) {
  await requireRole("agent", "supervisor", "tenant_admin");
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  const call = await orNotFound(callsApi.get(id));
  const rec = call.recording;

  return (
    <>
      <PageHeader
        title={`Call with ${call.caller_name ?? "an anonymous caller"}`}
        description={`${call.department_name} · ${formatDateTime(call.queued_at)}`}
        back={{ href: "/calls", label: "Call history" }}
        actions={<CallStatusBadge status={call.status} />}
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />

      <div className="space-y-6">
        <Card title="Summary">
          <DescriptionList
            items={[
              { label: "Caller", value: call.caller_name ?? "—" },
              { label: "Phone", value: call.caller_phone ?? "—" },
              { label: "Email", value: call.caller_email ?? "—" },
              { label: "Agent", value: call.agent_name ?? "—" },
              { label: "Answered", value: formatDateTime(call.answered_at) },
              { label: "Ended", value: formatDateTime(call.ended_at) },
              { label: "Duration", value: call.duration_seconds !== null ? formatDuration(call.duration_seconds) : "—" },
              { label: "Outcome", value: call.end_reason ? (endReasonLabels[call.end_reason] ?? call.end_reason) : "—" },
            ]}
          />
          {call.callback ? (
            <p className="mt-4 text-sm text-slate-700">
              The caller asked to be called back.{" "}
              <Link href={`/callbacks/${call.callback.id}`} className="font-medium text-indigo-600 hover:text-indigo-500">
                View callback
              </Link>{" "}
              <CallbackStatusBadge status={call.callback.status} />
            </p>
          ) : null}
        </Card>

        {call.permissions.play_recording || rec ? (
          <Card
            title="Recording"
            actions={rec ? <RecordingStatusBadge status={rec.status} /> : undefined}
          >
            {!rec ? (
              <p className="text-sm text-slate-600">
                {call.answered_at ? "No recording is available for this call." : "This call was not answered, so nothing was recorded."}
              </p>
            ) : rec.status === "ready" && call.permissions.play_recording ? (
              <div className="space-y-4">
                <p className="text-sm text-slate-600">
                  {rec.duration_seconds !== null ? formatDuration(rec.duration_seconds) : "—"} · {formatBytes(rec.size_bytes)}. Every play and
                  download is recorded in the audit log.
                </p>
                <RecordingPlayer access={recordingAccessAction.bind(null, call.id)} />
                {call.permissions.delete_recording ? (
                  <ConfirmButton
                    action={deleteRecordingAction.bind(null, call.id)}
                    confirmMessage="Delete this recording? It will be hidden from your organisation. The platform administrator can restore it."
                    variant="danger"
                    pendingLabel="Deleting…"
                  >
                    Delete recording
                  </ConfirmButton>
                ) : null}
              </div>
            ) : rec.status === "ready" ? (
              <p className="text-sm text-slate-600">A recording exists. You do not have permission to play it.</p>
            ) : rec.status === "failed" ? (
              <p className="text-sm text-slate-600">This call could not be recorded{rec.error ? `: ${rec.error}` : "."}</p>
            ) : (
              <p className="text-sm text-slate-600">The recording is still being processed. Reload this page in a moment.</p>
            )}
          </Card>
        ) : null}

        <Card title="Chat transcript">
          {call.messages.length === 0 ? (
            <p className="text-sm text-slate-500">No chat messages were sent during this call.</p>
          ) : (
            <ul className="space-y-3">
              {call.messages.map((m) => (
                <li key={m.id} className="text-sm">
                  <span className="font-medium text-slate-900">{m.name}</span>{" "}
                  <span className="text-xs text-slate-500">
                    {m.sender === "agent" ? "agent" : "caller"} · {formatDateTime(m.sent_at)}
                  </span>
                  <p className="mt-0.5 whitespace-pre-wrap break-words text-slate-700">{m.body}</p>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title="Routing timeline">
          <ol className="space-y-2 text-sm text-slate-700">
            <li>{formatDateTime(call.queued_at)} — caller joined the queue</li>
            {call.offers.map((o, i) => (
              <li key={i}>
                {formatDateTime(o.offered_at)} — offered to {o.agent_name ?? "a former agent"}:{" "}
                <span className="font-medium">{outcomeLabels[o.outcome] ?? o.outcome}</span>
              </li>
            ))}
            {call.answered_at ? <li>{formatDateTime(call.answered_at)} — answered</li> : null}
            {call.ended_at ? (
              <li>
                {formatDateTime(call.ended_at)} — ended{call.end_reason ? ` (${endReasonLabels[call.end_reason] ?? call.end_reason})` : ""}
              </li>
            ) : null}
          </ol>
        </Card>

        <Card title="Recording consent">
          <p className="text-sm text-slate-700">{call.consent_text}</p>
          <p className="mt-2 text-xs text-slate-500">The caller agreed on {formatDateTime(call.consent_given_at)}.</p>
        </Card>
      </div>
    </>
  );
}
