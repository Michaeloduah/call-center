"use client";

import Link from "next/link";
import { useActionState } from "react";

import { signupAction } from "@/app/(auth)/actions";
import { Alert } from "@/components/ui/Alert";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { TextField } from "@/components/ui/Field";
import { initialFormState } from "@/lib/forms";

export function SignupForm() {
  const [state, action] = useActionState(signupAction, initialFormState);

  if (state.status === "success") {
    return (
      <div className="space-y-4">
        <Alert tone="success">{state.message}</Alert>
        <p className="text-sm text-slate-600">
          Didn&apos;t get it?{" "}
          <Link href="/verify-email" className="font-medium text-indigo-600 hover:text-indigo-500">
            Resend verification email
          </Link>
        </p>
      </div>
    );
  }

  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextField
        name="company_name"
        label="Company name"
        autoComplete="organization"
        required
        defaultValue={state.values?.company_name}
        error={state.fieldErrors?.company_name}
      />
      <TextField
        name="full_name"
        label="Your name"
        autoComplete="name"
        required
        defaultValue={state.values?.full_name}
        error={state.fieldErrors?.full_name}
      />
      <TextField
        name="email"
        type="email"
        label="Work email"
        autoComplete="email"
        required
        defaultValue={state.values?.email}
        error={state.fieldErrors?.email}
      />
      <TextField
        name="password"
        type="password"
        label="Password"
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
      <SubmitButton className="w-full" pendingLabel="Creating account…">
        Create account
      </SubmitButton>
    </form>
  );
}
