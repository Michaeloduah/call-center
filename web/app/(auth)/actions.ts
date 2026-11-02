"use server";

import { redirect } from "next/navigation";

import { authApi } from "@/lib/api/auth";
import { type FormState, field, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { homePathFor, setSessionCookie } from "@/lib/server/session";

/** Only same-site relative paths are accepted as post-login destinations (prevents open redirects). */
function safeNext(next: string): string | null {
  return next.startsWith("/") && !next.startsWith("//") && !next.startsWith("/\\") ? next : null;
}

function passwordsMatch(formData: FormData): FormState | null {
  const password = String(formData.get("password") ?? "");
  if (password !== String(formData.get("password_confirmation") ?? "")) {
    return { status: "error", fieldErrors: { password_confirmation: "Passwords do not match." } };
  }
  return null;
}

export async function loginAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const email = field(formData, "email");
  const echo = values(formData, ["email"]);
  let destination: string;
  try {
    const result = await authApi.login(email, String(formData.get("password") ?? ""));
    await setSessionCookie(result.session_token, result.expires_at);
    destination = safeNext(field(formData, "next")) ?? homePathFor(result.user.role);
  } catch (err) {
    return toFormState(err, echo);
  }
  redirect(destination);
}

export async function signupAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const echo = values(formData, ["company_name", "full_name", "email"]);
  const mismatch = passwordsMatch(formData);
  if (mismatch) return { ...mismatch, values: echo };
  try {
    const result = await authApi.signup({
      company_name: echo.company_name,
      full_name: echo.full_name,
      email: echo.email,
      password: String(formData.get("password") ?? ""),
    });
    return {
      status: "success",
      values: { email: result.email },
      message: result.email_sent
        ? `We sent a verification link to ${result.email}. Open it to activate your account.`
        : "Your account was created, but we could not send the verification email. Use “Resend verification email” below.",
    };
  } catch (err) {
    return toFormState(err, echo);
  }
}

export async function verifyEmailAction(_prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await authApi.verifyEmail(field(formData, "token"));
  } catch (err) {
    return toFormState(err);
  }
  redirect("/login?notice=email-verified");
}

export async function resendVerificationAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const echo = values(formData, ["email"]);
  try {
    await authApi.resendVerification(echo.email);
  } catch (err) {
    return toFormState(err, echo);
  }
  return {
    status: "success",
    values: echo,
    message: "If that account exists and is not yet verified, a new verification link is on its way.",
  };
}

export async function forgotPasswordAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const echo = values(formData, ["email"]);
  try {
    await authApi.forgotPassword(echo.email);
  } catch (err) {
    return toFormState(err, echo);
  }
  return { status: "success", values: echo, message: "If an account exists for that email, a reset link is on its way." };
}

export async function resetPasswordAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const mismatch = passwordsMatch(formData);
  if (mismatch) return mismatch;
  try {
    await authApi.resetPassword(field(formData, "token"), String(formData.get("password") ?? ""));
  } catch (err) {
    return toFormState(err);
  }
  redirect("/login?notice=password-reset");
}

export async function acceptInviteAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const mismatch = passwordsMatch(formData);
  if (mismatch) return mismatch;
  try {
    await authApi.acceptInvite(field(formData, "token"), String(formData.get("password") ?? ""));
  } catch (err) {
    return toFormState(err);
  }
  redirect("/login?notice=invite-accepted");
}
