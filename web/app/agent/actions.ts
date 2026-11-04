"use server";

import { agentApi } from "@/lib/api/agent";
import type { ActionResult } from "@/lib/forms";
import { attempt } from "@/lib/server/actions";
import type { AcceptResult, AgentSession, AgentStatus, ChatMessage, MediaGrant } from "@/lib/types";

export async function sessionAction(): Promise<ActionResult<AgentSession>> {
  return attempt(() => agentApi.session());
}

export async function setStatusAction(status: AgentStatus): Promise<ActionResult> {
  return attempt(() => agentApi.setStatus(status));
}

export async function heartbeatAction(): Promise<ActionResult<{ status: AgentStatus }>> {
  return attempt(() => agentApi.heartbeat());
}

export async function acceptAction(offerId: string): Promise<ActionResult<AcceptResult>> {
  return attempt(() => agentApi.accept(offerId));
}

export async function declineAction(offerId: string): Promise<ActionResult> {
  return attempt(() => agentApi.decline(offerId));
}

export async function endCallAction(callId: string): Promise<ActionResult> {
  return attempt(() => agentApi.endCall(callId));
}

export async function mediaAction(callId: string): Promise<ActionResult<MediaGrant>> {
  return attempt(() => agentApi.media(callId));
}

export async function agentChatHistoryAction(callId: string): Promise<ActionResult<ChatMessage[]>> {
  return attempt(() => agentApi.messages(callId));
}

export async function sendAgentChatAction(callId: string, body: string): Promise<ActionResult<ChatMessage>> {
  return attempt(() => agentApi.sendMessage(callId, body));
}
