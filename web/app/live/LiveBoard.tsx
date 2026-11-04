"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { boardAction } from "@/app/live/actions";
import { Alert } from "@/components/ui/Alert";
import { AgentStateBadge, Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { formatDuration } from "@/lib/format";
import { useEventStream } from "@/lib/realtime/useEventStream";
import type { Board, BoardDepartment } from "@/lib/types";
import { useNow } from "@/lib/useNow";

// Events usually arrive in bursts (one change touches several rows), so reads are coalesced.
const REFRESH_DEBOUNCE_MS = 300;
// Safety net in case an event is ever missed.
const REFRESH_FALLBACK_MS = 30_000;

export function LiveBoard({ initial }: { initial: Board }) {
  const [board, setBoard] = useState(initial);
  const [failed, setFailed] = useState(false);
  const pending = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const now = useNow();

  const load = useCallback(async () => {
    const res = await boardAction();
    setFailed(!res.ok);
    if (res.ok) setBoard(res.data);
  }, []);
  const refreshSoon = useCallback(() => {
    clearTimeout(pending.current);
    pending.current = setTimeout(() => void load(), REFRESH_DEBOUNCE_MS);
  }, [load]);

  const connected = useEventStream("/api/live/events", refreshSoon);
  useEffect(() => {
    const timer = setInterval(() => void load(), REFRESH_FALLBACK_MS);
    return () => {
      clearInterval(timer);
      clearTimeout(pending.current);
    };
  }, [load]);

  if (board.departments.length === 0) {
    return <EmptyState title="No departments to show" description="Departments you supervise will appear here." />;
  }

  return (
    <div className="space-y-6">
      {!connected ? <Alert tone="warning">Reconnecting to live updates…</Alert> : null}
      {failed ? <Alert tone="error">We couldn&apos;t refresh the board. Showing the last known state.</Alert> : null}
      {board.departments.map((d) => (
        <DepartmentCard key={d.id} department={d} now={now} />
      ))}
    </div>
  );
}

function DepartmentCard({ department: d, now }: { department: BoardDepartment; now: number | null }) {
  const waited = d.oldest_queued_since && now ? (now - new Date(d.oldest_queued_since).getTime()) / 1000 : null;
  const available = d.agents.filter((a) => a.state === "idle").length;

  return (
    <Card
      title={d.name}
      actions={
        <div className="flex gap-2">
          <Badge tone={d.queued > 0 ? "amber" : "slate"}>
            {d.queued} waiting{waited !== null ? ` · oldest ${formatDuration(waited)}` : ""}
          </Badge>
          <Badge tone={available > 0 ? "green" : "red"}>{available} available</Badge>
        </div>
      }
    >
      <div className="grid gap-6 md:grid-cols-2">
        <section aria-label={`${d.name} agents`}>
          <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">Agents</h3>
          {d.agents.length === 0 ? (
            <p className="text-sm text-slate-500">No agents are assigned to this department.</p>
          ) : (
            <ul className="divide-y divide-slate-100">
              {d.agents.map((a) => (
                <li key={a.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                  <span className="truncate text-slate-900">{a.full_name}</span>
                  <AgentStateBadge state={a.state} />
                </li>
              ))}
            </ul>
          )}
        </section>
        <section aria-label={`${d.name} calls`}>
          <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">Calls in progress</h3>
          {d.calls.length === 0 ? (
            <p className="text-sm text-slate-500">No calls right now.</p>
          ) : (
            <ul className="divide-y divide-slate-100">
              {d.calls.map((c) => (
                <li key={c.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                  <span className="min-w-0 truncate text-slate-900">
                    {c.caller_name ?? "Caller"} <span className="text-slate-500">→ {c.agent_name ?? "—"}</span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2">
                    <Badge tone={c.status === "active" ? "indigo" : "amber"}>{c.status === "active" ? "Active" : "Ringing"}</Badge>
                    <span className="w-12 text-right font-mono text-xs text-slate-600">
                      {now ? formatDuration((now - new Date(c.since).getTime()) / 1000) : ""}
                    </span>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </Card>
  );
}
