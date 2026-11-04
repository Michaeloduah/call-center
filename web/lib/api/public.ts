import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { CallCreateResult, CallerCallView, ChatMessage, PublicWidgetConfig } from "@/lib/types";

const CALLER_TOKEN_HEADER = "X-Caller-Token";

export interface CallbackBody {
  name: string;
  phone: string;
  email: string;
  reason: string;
}

/** Anonymous caller endpoints. Calls are authorised by their caller token, never by a session. */
export const publicApi = {
  getWidget: (publicKey: string) =>
    apiFetch<PublicWidgetConfig>(`/v1/public/widgets/${encodeURIComponent(publicKey)}`, { auth: false }),
  createCall: (publicKey: string, body: { name: string; phone: string; email: string; consent: boolean }) =>
    apiFetch<CallCreateResult>(`/v1/public/widgets/${encodeURIComponent(publicKey)}/calls`, {
      method: "POST",
      body,
      auth: false,
    }),
  getCall: (callId: string, callerToken: string) =>
    apiFetch<CallerCallView>(`/v1/public/calls/${encodeURIComponent(callId)}`, {
      auth: false,
      headers: { [CALLER_TOKEN_HEADER]: callerToken },
    }),
  messages: (callId: string, callerToken: string) =>
    apiFetch<ChatMessage[]>(`/v1/public/calls/${encodeURIComponent(callId)}/messages`, {
      auth: false,
      headers: { [CALLER_TOKEN_HEADER]: callerToken },
    }),
  sendMessage: (callId: string, callerToken: string, body: string) =>
    apiFetch<ChatMessage>(`/v1/public/calls/${encodeURIComponent(callId)}/messages`, {
      method: "POST",
      body: { body },
      auth: false,
      headers: { [CALLER_TOKEN_HEADER]: callerToken },
    }),
  createCallback: (publicKey: string, body: CallbackBody) =>
    apiFetch<{ id: string }>(`/v1/public/widgets/${encodeURIComponent(publicKey)}/callbacks`, { method: "POST", body, auth: false }),
  createCallbackForCall: (callId: string, callerToken: string, body: CallbackBody) =>
    apiFetch<{ id: string }>(`/v1/public/calls/${encodeURIComponent(callId)}/callback`, {
      method: "POST",
      body,
      auth: false,
      headers: { [CALLER_TOKEN_HEADER]: callerToken },
    }),
  hangUp: (callId: string, callerToken: string) =>
    apiFetch<void>(`/v1/public/calls/${encodeURIComponent(callId)}/hangup`, {
      method: "POST",
      auth: false,
      headers: { [CALLER_TOKEN_HEADER]: callerToken },
    }),
};
