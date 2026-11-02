"use server";

import { publicApi } from "@/lib/api/public";
import { type FormState, field, values } from "@/lib/forms";
import { ApiError } from "@/lib/server/api";
import type { CallCreateResult, CallerCallView } from "@/lib/types";

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
