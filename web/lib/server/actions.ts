import "server-only";

import { redirect } from "next/navigation";

import type { FormState } from "@/lib/forms";
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
