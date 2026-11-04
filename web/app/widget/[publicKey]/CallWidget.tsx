"use client";

import { useRouter } from "next/navigation";
import { useActionState, useRef, useState, useTransition } from "react";

import { type CallbackTarget, type StartCallState, requestCallbackAction, startCallAction } from "@/app/widget/actions";
import { ChatPanel } from "@/components/chat/ChatPanel";
import { type FormState } from "@/lib/forms";
import { useCallSession } from "@/app/widget/[publicKey]/useCallSession";
import { weekdayLabels } from "@/lib/format";
import type { CallCreateResult, FieldMode, PublicWidgetConfig } from "@/lib/types";

const inputClass =
  "block w-full rounded-md border-0 px-3 py-2 text-sm text-slate-900 ring-1 ring-inset ring-slate-300 " +
  "focus:ring-2 focus:ring-inset focus:ring-slate-500 aria-[invalid=true]:ring-red-500";

/** The caller-facing call experience rendered inside the widget iframe. */
export function CallWidget({ config }: { config: PublicWidgetConfig }) {
  const [call, setCall] = useState<CallCreateResult | null>(null);
  // What the caller typed, so the callback form after a missed call is prefilled.
  const [typed, setTyped] = useState<Record<string, string>>({});

  return (
    <div className="flex min-h-screen flex-col bg-white">
      <header className="px-5 py-4 text-white" style={{ backgroundColor: config.accent_color }}>
        <p className="text-base font-semibold">{config.tenant_name}</p>
        <p className="text-sm opacity-90">{config.department_name}</p>
      </header>
      <main className="flex-1 px-5 py-5">
        {call ? (
          <ActiveCall key={call.call_id} call={call} config={config} typed={typed} onRestart={() => setCall(null)} />
        ) : config.availability === "closed" ? (
          <Closed config={config} />
        ) : config.availability === "unavailable" ? (
          <p className="text-sm text-slate-700">Calls to this team aren&apos;t available right now. Please try again later.</p>
        ) : config.availability === "no_agents" ? (
          <NoAgents config={config} />
        ) : (
          <StartForm
            config={config}
            onStarted={(started, values) => {
              setTyped(values);
              setCall(started);
            }}
          />
        )}
      </main>
    </div>
  );
}

function StartForm({
  config,
  onStarted,
}: {
  config: PublicWidgetConfig;
  onStarted: (call: CallCreateResult, values: Record<string, string>) => void;
}) {
  const [state, formAction] = useActionState(
    async (prev: StartCallState, formData: FormData) => {
      const next = await startCallAction(config.public_key, prev, formData);
      if (next.call) onStarted(next.call, { name: String(formData.get("name") ?? ""), phone: String(formData.get("phone") ?? ""), email: String(formData.get("email") ?? "") });
      return next;
    },
    { status: "idle" } as StartCallState,
  );
  const [micError, setMicError] = useState<string | null>(null);
  const [awaitingMic, setAwaitingMic] = useState(false);
  const [pending, startTransition] = useTransition();
  const form = config.caller_form;
  const mode = (m: FieldMode): FieldMode => (form.enabled ? m : "hidden");
  const e = state.fieldErrors ?? {};

  // Ask for the microphone before creating the call, so callers who refuse never join the queue.
  const onSubmit = async (ev: React.FormEvent<HTMLFormElement>) => {
    ev.preventDefault();
    const data = new FormData(ev.currentTarget);
    setMicError(null);
    // The browser may show a permission prompt here; reflect that until the caller answers it.
    setAwaitingMic(true);
    try {
      const probe = await navigator.mediaDevices.getUserMedia({ audio: true });
      probe.getTracks().forEach((t) => t.stop());
    } catch {
      setMicError("We need access to your microphone for the call. Allow it in your browser, then try again.");
      return;
    } finally {
      setAwaitingMic(false);
    }
    startTransition(() => formAction(data));
  };

  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      {config.greeting ? <p className="text-sm text-slate-700">{config.greeting}</p> : null}
      {micError ? <Notice tone="error">{micError}</Notice> : null}
      {state.status === "error" && state.message ? <Notice tone="error">{state.message}</Notice> : null}
      <Field label="Your name" name="name" mode={mode(form.name)} autoComplete="name" defaultValue={state.values?.name} error={e.name} />
      <Field label="Phone number" name="phone" type="tel" mode={mode(form.phone)} autoComplete="tel" defaultValue={state.values?.phone} error={e.phone} />
      <Field label="Email" name="email" type="email" mode={mode(form.email)} autoComplete="email" defaultValue={state.values?.email} error={e.email} />
      <div className="rounded-md bg-slate-50 p-3 ring-1 ring-slate-200">
        <label className="flex items-start gap-2 text-sm text-slate-700">
          <input type="checkbox" name="consent" required className="mt-0.5 h-4 w-4 rounded border-slate-300" aria-invalid={e.consent ? true : undefined} />
          <span>{config.consent_text}</span>
        </label>
        {e.consent ? <p className="mt-2 text-sm text-red-600">{e.consent}</p> : null}
      </div>
      <button
        type="submit"
        disabled={pending || awaitingMic}
        className="w-full rounded-md px-4 py-2.5 text-sm font-semibold text-white disabled:opacity-60"
        style={{ backgroundColor: config.accent_color }}
      >
        {awaitingMic ? "Waiting for microphone permission…" : pending ? "Starting call…" : "Start call"}
      </button>
      <p className="text-center text-xs text-slate-500">Uses your internet connection — no airtime charges.</p>
    </form>
  );
}

function Field({
  label, name, mode, error, ...props
}: { label: string; name: string; mode: FieldMode; error?: string } & React.ComponentProps<"input">) {
  if (mode === "hidden") return null;
  return (
    <div className="space-y-1">
      <label htmlFor={name} className="block text-sm font-medium text-slate-800">
        {label}
        {mode === "optional" ? <span className="font-normal text-slate-500"> (optional)</span> : null}
      </label>
      <input id={name} name={name} required={mode === "required"} aria-invalid={error ? true : undefined} className={inputClass} {...props} />
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
    </div>
  );
}

const endMessages: Record<string, string> = {
  caller_hung_up: "You ended the call.",
  agent_ended: "The agent ended the call. Thank you for calling.",
  agent_left: "The agent was disconnected and the call ended.",
  max_wait_exceeded: "Sorry, nobody was available to take your call. Please try again later.",
  connection_lost: "Your connection dropped and the call ended.",
  expired: "This call session has expired.",
};

function ActiveCall({
  call,
  config,
  typed,
  onRestart,
}: {
  call: CallCreateResult;
  config: PublicWidgetConfig;
  typed: Record<string, string>;
  onRestart: () => void;
}) {
  const audioRef = useRef<HTMLDivElement>(null);
  const session = useCallSession(call, audioRef);
  const [hangingUp, setHangingUp] = useState(false);

  return (
    <div className="space-y-5 text-center">
      <div ref={audioRef} hidden />
      {session.phase === "connecting" ? <Status title="Connecting…" detail="Setting up your call." /> : null}
      {session.phase === "waiting" ? (
        <Status
          title="Waiting for an agent"
          detail={
            session.agentRinging
              ? "An agent is being notified. Please stay on this page."
              : session.queuePosition
                ? `You are number ${session.queuePosition} in line. Please stay on this page.`
                : "Please stay on this page."
          }
          pulse
          color={config.accent_color}
        />
      ) : null}
      {session.phase === "in_call" ? (
        <>
          <Status title="Connected" detail="You're speaking with an agent." color={config.accent_color} />
          <p className="text-xs text-slate-500">This call is being recorded.</p>
          <ChatPanel
            messages={session.messages}
            me="caller"
            onSend={session.sendMessage}
            sending={session.sendingMessage}
            error={session.messageError}
            accent={config.accent_color}
          />
        </>
      ) : null}
      {session.phase === "error" ? <Notice tone="error">{session.error}</Notice> : null}
      {session.phase === "ended" ? (
        <Status title="Call ended" detail={endMessages[session.endReason ?? ""] ?? "Thank you for calling."} />
      ) : null}

      {session.phase === "waiting" || session.phase === "in_call" || session.phase === "connecting" ? (
        <div className="flex justify-center gap-3">
          {session.phase === "in_call" ? (
            <button type="button" onClick={() => void session.toggleMute()} className="rounded-full px-5 py-2.5 text-sm font-medium ring-1 ring-slate-300">
              {session.muted ? "Unmute" : "Mute"}
            </button>
          ) : null}
          <button
            type="button"
            disabled={hangingUp}
            onClick={async () => {
              setHangingUp(true);
              await session.hangUp();
            }}
            className="rounded-full bg-red-600 px-5 py-2.5 text-sm font-semibold text-white disabled:opacity-60"
          >
            {hangingUp ? "Ending…" : "Hang up"}
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          {session.endReason === "max_wait_exceeded" ? (
            <CallbackForm
              target={{ kind: "call", callId: call.call_id, callerToken: call.caller_token }}
              config={config}
              prefill={typed}
              intro="Leave your details and we'll call you back."
            />
          ) : null}
          <button type="button" onClick={onRestart} className="text-sm font-medium text-slate-700 underline">
            Start a new call
          </button>
        </div>
      )}
    </div>
  );
}

function Status({ title, detail, pulse = false, color }: { title: string; detail: string; pulse?: boolean; color?: string }) {
  return (
    <div className="space-y-3 py-6" role="status" aria-live="polite">
      <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full" style={{ backgroundColor: color ?? "#e2e8f0" }}>
        <span className={`h-4 w-4 rounded-full bg-white ${pulse ? "animate-ping" : ""}`} />
      </div>
      <p className="text-base font-semibold text-slate-900">{title}</p>
      <p className="text-sm text-slate-600">{detail}</p>
    </div>
  );
}

function Notice({ tone, children }: { tone: "error" | "success"; children: React.ReactNode }) {
  const style = tone === "error" ? "bg-red-50 text-red-800 ring-red-200" : "bg-green-50 text-green-800 ring-green-200";
  return (
    <div role={tone === "error" ? "alert" : "status"} className={`rounded-md px-3 py-2 text-left text-sm ring-1 ${style}`}>
      {children}
    </div>
  );
}

function NoAgents({ config }: { config: PublicWidgetConfig }) {
  const router = useRouter();
  const [checking, startChecking] = useTransition();
  return (
    <div className="space-y-4">
      <p className="text-sm text-slate-700">Nobody is available to take your call right now. Please try again shortly.</p>
      <button
        type="button"
        disabled={checking}
        onClick={() => startChecking(() => router.refresh())}
        className="rounded-md px-4 py-2 text-sm font-medium text-slate-800 ring-1 ring-slate-300 disabled:opacity-60"
      >
        {checking ? "Checking…" : "Check again"}
      </button>
      <CallbackForm target={{ kind: "widget", publicKey: config.public_key }} config={config} intro="Or leave your details and we'll call you back." />
    </div>
  );
}

function Closed({ config }: { config: PublicWidgetConfig }) {
  const byDay = weekdayLabels.map((label, day) => ({
    label,
    ranges: config.business_hours.filter((h) => h.weekday === day).map((h) => `${h.opens}–${h.closes}`),
  }));
  return (
    <div className="space-y-4">
      <p className="text-sm text-slate-700">We&apos;re closed right now. Please call back during our opening hours.</p>
      <dl className="grid grid-cols-[7rem_1fr] gap-y-1 text-sm">
        {byDay.map((d) => (
          <div key={d.label} className="contents">
            <dt className="text-slate-500">{d.label}</dt>
            <dd className="text-slate-900">{d.ranges.length ? d.ranges.join(", ") : "Closed"}</dd>
          </div>
        ))}
      </dl>
      <p className="text-xs text-slate-500">Times shown in {config.timezone.replaceAll("_", " ")}.</p>
      <CallbackForm target={{ kind: "widget", publicKey: config.public_key }} config={config} intro="Leave your details and we'll call you back when we open." />
    </div>
  );
}

/** Asks the caller to leave contact details when nobody could take their call. */
function CallbackForm({
  target,
  config,
  intro,
  prefill = {},
}: {
  target: CallbackTarget;
  config: PublicWidgetConfig;
  intro: string;
  prefill?: Record<string, string>;
}) {
  const [state, formAction, pending] = useActionState(
    (prev: FormState, formData: FormData) => requestCallbackAction(target, prev, formData),
    { status: "idle" } as FormState,
  );
  const e = state.fieldErrors ?? {};
  const v = state.values ?? prefill;

  if (state.status === "success") {
    return (
      <Notice tone="success">Thank you. We have your details and will call you back as soon as we can.</Notice>
    );
  }
  return (
    <form action={formAction} className="space-y-3 border-t border-slate-200 pt-4 text-left" noValidate>
      <p className="text-sm font-medium text-slate-800">{intro}</p>
      {state.status === "error" && state.message && Object.keys(e).length === 0 ? <Notice tone="error">{state.message}</Notice> : null}
      <Field label="Your name" name="name" mode="required" autoComplete="name" defaultValue={v.name} error={e.name} />
      <Field label="Phone number" name="phone" type="tel" mode="optional" autoComplete="tel" defaultValue={v.phone} error={e.phone} />
      <Field label="Email" name="email" type="email" mode="optional" autoComplete="email" defaultValue={v.email} error={e.email} />
      <Field label="What is it about?" name="reason" mode="optional" defaultValue={v.reason} error={e.reason} />
      <p className="text-xs text-slate-500">We only use these details to return your call. Please give a phone number or an email.</p>
      <button
        type="submit"
        disabled={pending}
        className="w-full rounded-md px-4 py-2.5 text-sm font-semibold text-white disabled:opacity-60"
        style={{ backgroundColor: config.accent_color }}
      >
        {pending ? "Sending…" : "Request a callback"}
      </button>
    </form>
  );
}
