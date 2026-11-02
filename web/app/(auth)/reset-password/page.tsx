import type { Metadata } from "next";
import Link from "next/link";

import { resetPasswordAction } from "@/app/(auth)/actions";
import { SetPasswordForm } from "@/app/(auth)/SetPasswordForm";
import { Alert } from "@/components/ui/Alert";
import { param } from "@/lib/format";

export const metadata: Metadata = { title: "Reset password · CallCenter" };

export default async function ResetPasswordPage({ searchParams }: PageProps<"/reset-password">) {
  const token = param((await searchParams).token);
  return (
    <>
      <h1 className="mb-6 text-xl font-semibold text-slate-900">Choose a new password</h1>
      {token ? (
        <SetPasswordForm
          token={token}
          action={resetPasswordAction}
          submitLabel="Change password"
          invalidTokenLink={{ href: "/forgot-password", label: "Request a new reset link" }}
        />
      ) : (
        <Alert tone="error">
          This reset link is incomplete.{" "}
          <Link href="/forgot-password" className="font-medium underline">
            Request a new one
          </Link>
          .
        </Alert>
      )}
    </>
  );
}
