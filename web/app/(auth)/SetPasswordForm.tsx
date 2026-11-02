"use client";

import Link from "next/link";
import { useActionState } from "react";

import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { TextField } from "@/components/ui/Field";
import { type FormState, initialFormState } from "@/lib/forms";

/** Password + confirmation form shared by password reset and invitation acceptance. */
export function SetPasswordForm({
  token,
  action: serverAction,
  submitLabel,
  invalidTokenLink,
}: {
  token: string;
  action: (prev: FormState, formData: FormData) => Promise<FormState>;
  submitLabel: string;
  invalidTokenLink: { href: string; label: string };
}) {
  const [state, action] = useActionState(serverAction, initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      {state.code === "invalid_token" ? (
        <p className="text-sm">
          <Link href={invalidTokenLink.href} className="font-medium text-indigo-600 hover:text-indigo-500">
            {invalidTokenLink.label}
          </Link>
        </p>
      ) : null}
      <input type="hidden" name="token" value={token} />
      <TextField
        name="password"
        type="password"
        label="New password"
        autoComplete="new-password"
        required
        hint="At least 10 characters."
        error={state.fieldErrors?.password}
      />
      <TextField
        name="password_confirmation"
        type="password"
        label="Confirm password"
        autoComplete="new-password"
        required
        error={state.fieldErrors?.password_confirmation}
      />
      <SubmitButton className="w-full">{submitLabel}</SubmitButton>
    </form>
  );
}
