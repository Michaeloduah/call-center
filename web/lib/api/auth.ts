import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { InviteInfo, LoginResult, SignupResult } from "@/lib/types";

export const authApi = {
  login: (email: string, password: string) =>
    apiFetch<LoginResult>("/v1/auth/login", { method: "POST", body: { email, password }, auth: false }),
  logout: () => apiFetch<void>("/v1/auth/logout", { method: "POST" }),
  signup: (body: { company_name: string; full_name: string; email: string; password: string }) =>
    apiFetch<SignupResult>("/v1/auth/signup", { method: "POST", body, auth: false }),
  verifyEmail: (token: string) =>
    apiFetch<void>("/v1/auth/verify-email", { method: "POST", body: { token }, auth: false }),
  resendVerification: (email: string) =>
    apiFetch<void>("/v1/auth/resend-verification", { method: "POST", body: { email }, auth: false }),
  forgotPassword: (email: string) =>
    apiFetch<void>("/v1/auth/forgot-password", { method: "POST", body: { email }, auth: false }),
  resetPassword: (token: string, password: string) =>
    apiFetch<void>("/v1/auth/reset-password", { method: "POST", body: { token, password }, auth: false }),
  lookupInvite: (token: string) =>
    apiFetch<InviteInfo>("/v1/auth/invites/lookup", { method: "POST", body: { token }, auth: false }),
  acceptInvite: (token: string, password: string) =>
    apiFetch<void>("/v1/auth/invites/accept", { method: "POST", body: { token, password }, auth: false }),
};
