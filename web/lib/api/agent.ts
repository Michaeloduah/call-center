import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { AcceptResult, AgentSession, AgentStatus, Board, ChatMessage, MediaGrant } from "@/lib/types";

const offerPath = (id: string, action: "accept" | "decline") => `/v1/agent/offers/${encodeURIComponent(id)}/${action}`;
const callPath = (id: string, action: "media" | "end") => `/v1/agent/calls/${encodeURIComponent(id)}/${action}`;

export const agentApi = {
  session: () => apiFetch<AgentSession>("/v1/agent/session"),
  setStatus: (status: AgentStatus) => apiFetch<void>("/v1/agent/presence", { method: "PUT", body: { status } }),
  heartbeat: () => apiFetch<{ status: AgentStatus }>("/v1/agent/presence/heartbeat", { method: "POST" }),
  accept: (offerId: string) => apiFetch<AcceptResult>(offerPath(offerId, "accept"), { method: "POST" }),
  decline: (offerId: string) => apiFetch<void>(offerPath(offerId, "decline"), { method: "POST" }),
  media: (callId: string) => apiFetch<MediaGrant>(callPath(callId, "media")),
  messages: (callId: string) => apiFetch<ChatMessage[]>(`/v1/agent/calls/${encodeURIComponent(callId)}/messages`),
  sendMessage: (callId: string, body: string) =>
    apiFetch<ChatMessage>(`/v1/agent/calls/${encodeURIComponent(callId)}/messages`, { method: "POST", body: { body } }),
  endCall: (callId: string) => apiFetch<void>(callPath(callId, "end"), { method: "POST" }),
};

export const liveApi = {
  board: () => apiFetch<Board>("/v1/live/board"),
};
