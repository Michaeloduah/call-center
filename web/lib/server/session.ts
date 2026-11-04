import "server-only";

import { cache } from "react";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { ApiError, SESSION_COOKIE, apiFetch } from "@/lib/server/api";
import type { Me, Role } from "@/lib/types";

/** The current user, or null when signed out. Deduplicated per request. */
export const getCurrentUser = cache(async (): Promise<Me | null> => {
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  if (!token) return null;
  try {
    return await apiFetch<Me>("/v1/auth/me");
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return null;
    throw err;
  }
});

/** Where each role lands after signing in. */
export function homePathFor(role: Role): string {
  switch (role) {
    case "platform_admin":
      return "/platform/tenants";
    case "tenant_admin":
      return "/admin";
    case "supervisor":
      return "/live";
    case "agent":
      return "/agent";
  }
}

/**
 * Ensures a signed-in user with one of the given roles. Signed-out users go to the login page;
 * users with the wrong role go to their own home. The API enforces the same rules independently.
 */
export async function requireRole(...roles: Role[]): Promise<Me> {
  const user = await getCurrentUser();
  if (!user) redirect("/login?expired=1");
  if (!roles.includes(user.role)) redirect(homePathFor(user.role));
  return user;
}

export async function setSessionCookie(token: string, expiresAt: string): Promise<void> {
  (await cookies()).set(SESSION_COOKIE, token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    expires: new Date(expiresAt),
  });
}

export async function clearSessionCookie(): Promise<void> {
  (await cookies()).delete(SESSION_COOKIE);
}
