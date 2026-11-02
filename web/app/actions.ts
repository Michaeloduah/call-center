"use server";

import { redirect } from "next/navigation";

import { authApi } from "@/lib/api/auth";
import { ApiError } from "@/lib/server/api";
import { clearSessionCookie } from "@/lib/server/session";

export async function logoutAction(): Promise<void> {
  try {
    await authApi.logout();
  } catch (err) {
    // An already-expired session is fine: the goal is to be signed out.
    if (!(err instanceof ApiError && err.status === 401)) throw err;
  }
  await clearSessionCookie();
  redirect("/login?notice=signed-out");
}
