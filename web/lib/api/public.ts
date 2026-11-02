import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { CallCreateResult, CallerCallView, PublicWidgetConfig } from "@/lib/types";

const CALLER_TOKEN_HEADER = "X-Caller-Token";

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
  hangUp: (callId: string, callerToken: string) =>
    apiFetch<void>(`/v1/public/calls/${encodeURIComponent(callId)}/hangup`, {
      method: "POST",
      auth: false,
      headers: { [CALLER_TOKEN_HEADER]: callerToken },
    }),
};
