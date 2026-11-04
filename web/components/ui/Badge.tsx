import {
  agentStateLabels,
  callStatusLabels,
  callbackStatusLabels,
  recordingStatusLabels,
  subscriptionStatusLabels,
  tenantStatusLabels,
  userStatusLabels,
} from "@/lib/format";
import type { AgentState, CallStatus, CallbackStatus, RecordingStatus, SubscriptionStatus, TenantStatus, UserStatus } from "@/lib/types";

const tones = {
  green: "bg-green-50 text-green-700 ring-green-600/20",
  amber: "bg-amber-50 text-amber-800 ring-amber-600/20",
  red: "bg-red-50 text-red-700 ring-red-600/20",
  slate: "bg-slate-50 text-slate-700 ring-slate-500/20",
  indigo: "bg-indigo-50 text-indigo-700 ring-indigo-600/20",
};

export function Badge({ tone = "slate", children }: { tone?: keyof typeof tones; children: React.ReactNode }) {
  return (
    <span className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${tones[tone]}`}>
      {children}
    </span>
  );
}

const userTone: Record<UserStatus, keyof typeof tones> = { active: "green", invited: "amber", disabled: "red" };
const tenantTone: Record<TenantStatus, keyof typeof tones> = { active: "green", pending_verification: "amber", suspended: "red" };

export function UserStatusBadge({ status }: { status: UserStatus }) {
  return <Badge tone={userTone[status]}>{userStatusLabels[status]}</Badge>;
}

export function TenantStatusBadge({ status }: { status: TenantStatus }) {
  return <Badge tone={tenantTone[status]}>{tenantStatusLabels[status]}</Badge>;
}

const agentTone: Record<AgentState, keyof typeof tones> = {
  idle: "green",
  ringing: "amber",
  on_call: "indigo",
  away: "slate",
  offline: "slate",
};

export function AgentStateBadge({ state }: { state: AgentState }) {
  return <Badge tone={agentTone[state]}>{agentStateLabels[state]}</Badge>;
}

const callTone: Record<CallStatus, keyof typeof tones> = {
  queued: "amber",
  ringing: "amber",
  active: "indigo",
  completed: "green",
  abandoned: "slate",
  missed: "red",
  failed: "red",
};

export function CallStatusBadge({ status }: { status: CallStatus }) {
  return <Badge tone={callTone[status]}>{callStatusLabels[status]}</Badge>;
}

const callbackTone: Record<CallbackStatus, keyof typeof tones> = { open: "amber", claimed: "indigo", completed: "green", cancelled: "slate" };

export function CallbackStatusBadge({ status }: { status: CallbackStatus }) {
  return <Badge tone={callbackTone[status]}>{callbackStatusLabels[status]}</Badge>;
}

const recordingTone: Record<RecordingStatus, keyof typeof tones> = { starting: "amber", recording: "amber", ready: "green", failed: "red" };

export function RecordingStatusBadge({ status }: { status: RecordingStatus }) {
  return <Badge tone={recordingTone[status]}>{recordingStatusLabels[status]}</Badge>;
}

const subscriptionTone: Record<SubscriptionStatus, keyof typeof tones> = { active: "green", past_due: "red", cancelled: "slate", incomplete: "amber" };

export function SubscriptionStatusBadge({ status }: { status: SubscriptionStatus }) {
  return <Badge tone={subscriptionTone[status]}>{subscriptionStatusLabels[status]}</Badge>;
}
