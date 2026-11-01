import type { Metadata } from "next";
import Link from "next/link";

import { ConfirmEmailForm, ResendVerificationForm } from "@/app/(auth)/verify-email/VerifyForms";
import { param } from "@/lib/format";

export const metadata: Metadata = { title: "Verify email · CallCenter" };

export default async function VerifyEmailPage({ searchParams }: PageProps<"/verify-email">) {
  const token = param((await searchParams).token);
  return (
    <>
      <h1 className="text-xl font-semibold text-slate-900">{token ? "Confirm your email" : "Resend verification email"}</h1>
      <p className="mb-6 mt-1 text-sm text-slate-600">
        {token
          ? "Click the button below to verify your email address and activate your account."
          : "Enter the email you signed up with and we'll send a new verification link."}
      </p>
      {token ? <ConfirmEmailForm token={token} /> : <ResendVerificationForm />}
      <p className="mt-6 text-center text-sm text-slate-600">
        <Link href="/login" className="font-medium text-indigo-600 hover:text-indigo-500">
          Back to sign in
        </Link>
      </p>
    </>
  );
}
