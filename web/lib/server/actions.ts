import "server-only";

import { redirect } from "next/navigation";

import type { ActionResult, FormState } from "@/lib/forms";
import { ApiError } from "@/lib/server/api";

/**
 * Converts an error thrown while calling the API into form state. Expired sessions are sent to
 * the login page; anything unexpected is rethrown so the route's error boundary handles it.
 */
export function toFormState(err: unknown, values?: Record<string, string>): FormState {
  if (err instanceof ApiError) {
    if (err.status === 401 && err.code === "unauthenticated") redirect("/login?expired=1");
    return { status: "error", message: err.message, code: err.code, fieldErrors: err.fields, values };
  }
  throw err;
}

/**
 * Runs an API call for a client component that invokes a server action directly. API errors become
 * a result the UI can react to; expired sessions go to the login page; anything else is rethrown.
 */
export async function attempt<T>(request: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, data: await request() };
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 401 && err.code === "unauthenticated") redirect("/login?expired=1");
      return { ok: false, code: err.code, message: err.message };
    }
    throw err;
  }
}
