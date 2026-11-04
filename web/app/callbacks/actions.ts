"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { callbacksApi } from "@/lib/api/calls";
import { type FormState, field, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { NoticeKey } from "@/lib/notices";

const detail = (id: string) => `/callbacks/${id}`;

/** Runs a one-click workflow step, then returns to the callback with a notice or an error code. */
async function step(id: string, notice: NoticeKey, run: () => Promise<void>): Promise<never> {
  try {
    await run();
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) redirect(`${detail(id)}?error=${encodeURIComponent(err.code)}`);
    throw err;
  }
  revalidatePath("/callbacks");
  redirect(`${detail(id)}?notice=${notice}`);
}

export async function claimCallbackAction(id: string): Promise<void> {
  await step(id, "callback-claimed", () => callbacksApi.claim(id));
}

export async function releaseCallbackAction(id: string): Promise<void> {
  await step(id, "callback-released", () => callbacksApi.release(id));
}

export async function cancelCallbackAction(id: string): Promise<void> {
  await step(id, "callback-cancelled", () => callbacksApi.cancel(id));
}

export async function completeCallbackAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await callbacksApi.complete(id, field(formData, "notes"));
  } catch (err) {
    return toFormState(err, values(formData, ["notes"]));
  }
  revalidatePath("/callbacks");
  redirect(`${detail(id)}?notice=callback-completed`);
}

export async function assignCallbackAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await callbacksApi.assign(id, field(formData, "user_id"));
  } catch (err) {
    return toFormState(err, values(formData, ["user_id"]));
  }
  revalidatePath("/callbacks");
  redirect(`${detail(id)}?notice=callback-assigned`);
}
