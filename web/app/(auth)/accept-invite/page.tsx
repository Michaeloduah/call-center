import type { Metadata } from "next";
import Link from "next/link";

import { acceptInviteAction } from "@/app/(auth)/actions";
import { SetPasswordForm } from "@/app/(auth)/SetPasswordForm";
import { Alert } from "@/components/ui/Alert";
import { authApi } from "@/lib/api/auth";
import { param } from "@/lib/format";
import { ApiError } from "@/lib/server/api";
import type { InviteInfo } from "@/lib/types";

export const metadata: Metadata = { title: "Accept invitation · CallCenter" };

async function lookup(token: string): Promise<InviteInfo | null> {
  try {
    return await authApi.lookupInvite(token);
  } catch (err) {
    if (err instanceof ApiError && (err.code === "invalid_token" || err.status === 422)) return null;
    throw err;
  }
}

export default async function AcceptInvitePage({ searchParams }: PageProps<"/accept-invite">) {
  const token = param((await searchParams).token);
  const invite = token ? await lookup(token) : null;

  if (!invite) {
    return (
      <>
        <h1 className="mb-6 text-xl font-semibold text-slate-900">Invitation unavailable</h1>
        <Alert tone="error">
          This invitation link is invalid, has expired, or has already been used. Ask your administrator to send a new one.
        </Alert>
        <p className="mt-6 text-center text-sm">
          <Link href="/login" className="font-medium text-indigo-600 hover:text-indigo-500">
            Go to sign in
          </Link>
        </p>
      </>
    );
  }

  return (
    <>
      <h1 className="text-xl font-semibold text-slate-900">Welcome, {invite.full_name}</h1>
      <p className="mb-6 mt-1 text-sm text-slate-600">
        You&apos;ve been invited to {invite.tenant_name ?? "CallCenter"}. Set a password for{" "}
        <span className="font-medium text-slate-900">{invite.email}</span> to finish setting up your account.
      </p>
      <SetPasswordForm
        token={token}
        action={acceptInviteAction}
        submitLabel="Create my account"
        invalidTokenLink={{ href: "/login", label: "Go to sign in" }}
      />
    </>
  );
}
