"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  acceptAction,
  declineAction,
  endCallAction,
  heartbeatAction,
  mediaAction,
  sessionAction,
  setStatusAction,
} from "@/app/agent/actions";
import { useRingtone } from "@/app/agent/useRingtone";
import { Alert } from "@/components/ui/Alert";
import { AgentStateBadge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, DescriptionList } from "@/components/ui/Card";
import { formatDuration } from "@/lib/format";
import { type MediaCredentials, useLiveKitRoom } from "@/lib/media/useLiveKitRoom";
import { useEventStream } from "@/lib/realtime/useEventStream";
import type { AgentSession, AgentStatus } from "@/lib/types";
import { useNow } from "@/lib/useNow";

type Message = { tone: "error" | "warning" | "info"; text: string };

const MIC_DENIED = "Allow microphone access in your browser to take calls, then try again.";

const choices: { status: AgentStatus; label: string; hint: string }[] = [
  { status: "available", label: "Available", hint: "Receive calls" },
  { status: "away", label: "Away", hint: "Pause incoming calls" },
  { status: "offline", label: "Offline", hint: "Stop receiving calls" },
];

/** The agent's workspace: availability, the incoming call and the call in progress. */
export function AgentConsole({ initial }: { initial: AgentSession }) {
  const [session, setSession] = useState(initial);
  const [message, setMessage] = useState<Message | null>(null);
  const [busy, setBusy] = useState(false);
  // Credentials are kept together with the call they belong to, so a finished call's room is never rejoined.
  const [grant, setGrant] = useState<{ callId: string; credentials: MediaCredentials } | null>(null);
  const requestedFor = useRef<string | null>(null);
  const previousCallId = useRef<string | null>(initial.call?.id ?? null);
  const audioRef = useRef<HTMLDivElement>(null);
  const now = useNow();

  const callId = session.call?.id ?? null;
  const credentials = grant && grant.callId === callId ? grant.credentials : null;

  const { unlock } = useRingtone(session.state === "ringing");
  const { muted, toggleMute } = useLiveKitRoom({
    credentials,
    audioContainer: audioRef,
    onConnectFailed: () =>
      setMessage({
        tone: "error",
        text: "We couldn't start your microphone or connect the call audio, so the call ended and you were set offline. Check your microphone permission, then go available again.",
      }),
    onLost: () => setMessage({ tone: "warning", text: "The audio connection was lost. The call ends automatically if it does not recover." }),
  });

  const apply = useCallback((next: AgentSession) => {
    // Keep an error that explains why the call ended (for example a microphone failure).
    if (previousCallId.current && !next.call) setMessage((m) => (m?.tone === "error" ? m : { tone: "info", text: "The call has ended." }));
    previousCallId.current = next.call?.id ?? null;
    setSession(next);
  }, []);
  const refresh = useCallback(async () => {
    const res = await sessionAction();
    if (res.ok) apply(res.data);
  }, [apply]);
  const streamConnected = useEventStream("/api/agent/events", refresh);

  // Tell the server this console is still open, and learn if it marked us offline meanwhile.
  const { status, state, call, offer, heartbeat_interval_seconds: heartbeatSeconds } = session;
  useEffect(() => {
    if (status === "offline") return;
    const timer = setInterval(async () => {
      const res = await heartbeatAction();
      if (res.ok && res.data.status !== status) void refresh();
    }, heartbeatSeconds * 1000);
    return () => clearInterval(timer);
  }, [status, heartbeatSeconds, refresh]);

  // Fetch credentials for the active call when we do not have them, for example after a page reload.
  useEffect(() => {
    if (!callId || requestedFor.current === callId) return;
    requestedFor.current = callId;
    void mediaAction(callId).then((res) => {
      if (res.ok) setGrant({ callId, credentials: { url: res.data.media_url, token: res.data.media_token } });
      else setMessage({ tone: "error", text: res.message });
    });
  }, [callId]);

  const changeStatus = async (next: AgentStatus) => {
    setMessage(null);
    if (next === "available") {
      try {
        // An agent who cannot capture audio must not be offered calls.
        const probe = await navigator.mediaDevices.getUserMedia({ audio: true });
        probe.getTracks().forEach((t) => t.stop());
      } catch {
        setMessage({ tone: "error", text: MIC_DENIED });
        return;
      }
      unlock();
    }
    setBusy(true);
    const res = await setStatusAction(next);
    if (!res.ok) setMessage({ tone: "error", text: res.message });
    await refresh();
    setBusy(false);
  };

  const accept = async () => {
    if (!offer) return;
    setBusy(true);
    setMessage(null);
    const res = await acceptAction(offer.id);
    if (res.ok) {
      requestedFor.current = res.data.call.id;
      setGrant({ callId: res.data.call.id, credentials: { url: res.data.media_url, token: res.data.media_token } });
      apply({ ...session, state: "on_call", offer: null, call: res.data.call });
    } else {
      setMessage({
        tone: "warning",
        text: res.code === "offer_expired" ? "That call was already answered or the caller hung up." : res.message,
      });
      await refresh();
    }
    setBusy(false);
  };

  const decline = async () => {
    if (!offer) return;
    setBusy(true);
    const res = await declineAction(offer.id);
    if (!res.ok && res.code !== "offer_expired") setMessage({ tone: "error", text: res.message });
    await refresh();
    setBusy(false);
  };

  const endCall = async () => {
    if (!call) return;
    setBusy(true);
    const res = await endCallAction(call.id);
    // Not found means the caller already hung up: either way the call is over.
    if (!res.ok && res.code !== "not_found") setMessage({ tone: "error", text: res.message });
    await refresh();
    setBusy(false);
  };

  const secondsLeft = offer && now ? Math.max(0, Math.ceil((new Date(offer.expires_at).getTime() - now) / 1000)) : null;
  const callSeconds = call && now ? (now - new Date(call.answered_at).getTime()) / 1000 : null;

  return (
    <div className="space-y-6">
      <div ref={audioRef} hidden />
      {!streamConnected ? <Alert tone="warning">Reconnecting to live updates… Incoming calls may be delayed.</Alert> : null}
      {message ? <Alert tone={message.tone}>{message.text}</Alert> : null}
      {state === "away" && !message ? (
        <Alert tone="info">You&apos;re away, so no calls are offered to you. Choose Available when you&apos;re ready.</Alert>
      ) : null}

      <Card
        title="Your availability"
        actions={<AgentStateBadge state={state} />}
      >
        <div role="group" aria-label="Availability" className="flex flex-wrap gap-2">
          {choices.map((c) => (
            <Button
              key={c.status}
              type="button"
              variant={status === c.status ? "primary" : "secondary"}
              aria-pressed={status === c.status}
              disabled={busy || status === c.status}
              onClick={() => void changeStatus(c.status)}
              title={c.hint}
            >
              {c.label}
            </Button>
          ))}
        </div>
        <p className="mt-3 text-sm text-slate-500">
          {status === "offline"
            ? "Go available to start receiving calls. Your browser will ask for microphone access."
            : "Keep this page open to receive calls."}
        </p>
      </Card>

      {offer ? (
        <section aria-live="assertive" className="rounded-lg bg-amber-50 p-5 ring-2 ring-amber-400">
          <h2 className="text-lg font-semibold text-amber-950">Incoming call</h2>
          <p className="mt-1 text-sm text-amber-900">
            {offer.caller_name ?? "A caller"} · {offer.department_name}
            {secondsLeft !== null ? ` · ${secondsLeft}s to answer` : ""}
          </p>
          <div className="mt-4 flex gap-3">
            <Button type="button" onClick={() => void accept()} disabled={busy}>
              Accept
            </Button>
            <Button type="button" variant="secondary" onClick={() => void decline()} disabled={busy}>
              Decline
            </Button>
          </div>
        </section>
      ) : null}

      {call ? (
        <Card title="Call in progress" actions={callSeconds !== null ? <span className="font-mono text-sm text-slate-700">{formatDuration(callSeconds)}</span> : null}>
          <DescriptionList
            items={[
              { label: "Caller", value: call.caller_name ?? "—" },
              { label: "Department", value: call.department_name },
              { label: "Phone", value: call.caller_phone ?? "—" },
              { label: "Email", value: call.caller_email ?? "—" },
            ]}
          />
          <div className="mt-5 flex gap-3">
            <Button type="button" variant="secondary" onClick={() => void toggleMute()} disabled={!credentials}>
              {muted ? "Unmute" : "Mute"}
            </Button>
            <Button type="button" variant="danger" onClick={() => void endCall()} disabled={busy}>
              End call
            </Button>
          </div>
        </Card>
      ) : null}

      {!offer && !call && state === "idle" ? (
        <p className="text-center text-sm text-slate-500">Waiting for the next call…</p>
      ) : null}
    </div>
  );
}
