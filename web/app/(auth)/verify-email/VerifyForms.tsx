"use client";

import Link from "next/link";
import { useActionState } from "react";

import { resendVerificationAction, verifyEmailAction } from "@/app/(auth)/actions";
import { Alert } from "@/components/ui/Alert";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { TextField } from "@/components/ui/Field";
import { initialFormState } from "@/lib/forms";

/** Verification needs an explicit click so link scanners in mail clients cannot consume the token. */
export function ConfirmEmailForm({ token }: { token: string }) {
  const [state, action] = useActionState(verifyEmailAction, initialFormState);
  return (
    <form action={action} className="space-y-5">
      <FormError state={state} />
      {state.code === "invalid_token" ? (
        <p className="text-sm text-slate-600">
          <Link href="/verify-email" className="font-medium text-indigo-600 hover:text-indigo-500">
            Request a new verification link
          </Link>
        </p>
      ) : null}
      <input type="hidden" name="token" value={token} />
      <SubmitButton className="w-full" pendingLabel="Verifying…">
        Verify my email
      </SubmitButton>
    </form>
  );
}

export function ResendVerificationForm() {
  const [state, action] = useActionState(resendVerificationAction, initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      {state.status === "success" ? <Alert tone="success">{state.message}</Alert> : <FormError state={state} />}
      <TextField
        name="email"
        type="email"
        label="Email"
        autoComplete="email"
        required
        defaultValue={state.values?.email}
        error={state.fieldErrors?.email}
      />
      <SubmitButton className="w-full" pendingLabel="Sending…">
        Resend verification email
      </SubmitButton>
    </form>
  );
}
