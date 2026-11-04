"use server";

import { type CallbackBody, publicApi } from "@/lib/api/public";
import { type FormState, field, values } from "@/lib/forms";
import { attempt, toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { ActionResult } from "@/lib/forms";
import type { CallCreateResult, CallerCallView, ChatMessage } from "@/lib/types";

/** Result of starting a call: form state plus, on success, the caller's call credentials. */
export interface StartCallState extends FormState {
  call?: CallCreateResult;
}

function toState(err: unknown, echo?: Record<string, string>): StartCallState {
  if (err instanceof ApiError) {
    return { status: "error", code: err.code, message: err.message, fieldErrors: err.fields, values: echo };
  }
  throw err;
}

export async function startCallAction(publicKey: string, _prev: StartCallState, formData: FormData): Promise<StartCallState> {
  const echo = values(formData, ["name", "phone", "email"]);
  try {
    const call = await publicApi.createCall(publicKey, {
      name: echo.name,
      phone: echo.phone,
      email: echo.email,
      consent: field(formData, "consent") === "on",
    });
    return { status: "success", call };
  } catch (err) {
    return toState(err, echo);
  }
}

export type CallStatusResult = { ok: true; call: CallerCallView } | { ok: false; code: string };

export async function callStatusAction(callId: string, callerToken: string): Promise<CallStatusResult> {
  try {
    return { ok: true, call: await publicApi.getCall(callId, callerToken) };
  } catch (err) {
    if (err instanceof ApiError) return { ok: false, code: err.code };
    throw err;
  }
}

export async function hangUpAction(callId: string, callerToken: string): Promise<{ ok: boolean }> {
  try {
    await publicApi.hangUp(callId, callerToken);
    return { ok: true };
  } catch (err) {
    if (err instanceof ApiError) return { ok: false };
    throw err;
  }
}

export async function chatHistoryAction(callId: string, callerToken: string): Promise<ActionResult<ChatMessage[]>> {
  return attempt(() => publicApi.messages(callId, callerToken));
}

export async function sendChatAction(callId: string, callerToken: string, body: string): Promise<ActionResult<ChatMessage>> {
  return attempt(() => publicApi.sendMessage(callId, callerToken, body));
}

/** Where a callback request is filed: from the widget itself, or for a call nobody answered. */
export type CallbackTarget = { kind: "widget"; publicKey: string } | { kind: "call"; callId: string; callerToken: string };

export async function requestCallbackAction(target: CallbackTarget, _prev: FormState, formData: FormData): Promise<FormState> {
  const echo = values(formData, ["name", "phone", "email", "reason"]);
  const body: CallbackBody = { name: echo.name, phone: echo.phone, email: echo.email, reason: echo.reason };
  try {
    if (target.kind === "widget") await publicApi.createCallback(target.publicKey, body);
    else await publicApi.createCallbackForCall(target.callId, target.callerToken, body);
    return { status: "success" };
  } catch (err) {
    return toFormState(err, echo);
  }
}
