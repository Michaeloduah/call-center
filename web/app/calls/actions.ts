"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { callsApi, platformRecordingsApi } from "@/lib/api/calls";
import type { ActionResult } from "@/lib/forms";
import { attempt } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { AccessGrant } from "@/lib/types";

/** Issues a short-lived playback or download link (audit-logged by the API). */
export async function recordingAccessAction(callId: string, download: boolean): Promise<ActionResult<AccessGrant>> {
  return attempt(() => callsApi.recordingAccess(callId, download));
}

export async function platformRecordingAccessAction(recordingId: string, download: boolean): Promise<ActionResult<AccessGrant>> {
  return attempt(() => platformRecordingsApi.access(recordingId, download));
}

export async function deleteRecordingAction(callId: string): Promise<void> {
  try {
    await callsApi.deleteRecording(callId);
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) redirect(`/calls/${callId}?error=${encodeURIComponent(err.code)}`);
    throw err;
  }
  revalidatePath(`/calls/${callId}`);
  redirect(`/calls/${callId}?notice=recording-deleted`);
}

export async function restoreRecordingAction(recordingId: string): Promise<void> {
  try {
    await platformRecordingsApi.restore(recordingId);
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) redirect(`/platform/recordings/deleted?error=${encodeURIComponent(err.code)}`);
    throw err;
  }
  revalidatePath("/platform/recordings/deleted");
  redirect("/platform/recordings/deleted?notice=recording-restored");
}
