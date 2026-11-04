"use client";

import { useActionState } from "react";

import { FormError } from "@/components/ui/FormError";
import { SelectField, TextAreaField } from "@/components/ui/Field";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { type FormState, initialFormState } from "@/lib/forms";

type Action = (prev: FormState, formData: FormData) => Promise<FormState>;

export function CompleteForm({ action }: { action: Action }) {
  const [state, formAction] = useActionState(action, initialFormState);
  return (
    <form action={formAction} className="space-y-4">
      <FormError state={state} />
      <TextAreaField
        name="notes"
        label="Notes (optional)"
        rows={3}
        maxLength={2000}
        defaultValue={state.values?.notes}
        error={state.fieldErrors?.notes}
        hint="What happened when you called back?"
      />
      <SubmitButton pendingLabel="Saving…">Mark as completed</SubmitButton>
    </form>
  );
}

export function AssignForm({ action, members }: { action: Action; members: { id: string; full_name: string }[] }) {
  const [state, formAction] = useActionState(action, initialFormState);
  return (
    <form action={formAction} className="space-y-4">
      <FormError state={state} />
      <SelectField
        name="user_id"
        label="Assign to"
        defaultValue={state.values?.user_id ?? ""}
        error={state.fieldErrors?.user_id}
        options={[{ value: "", label: "Choose an agent…" }, ...members.map((m) => ({ value: m.id, label: m.full_name }))]}
      />
      <SubmitButton variant="secondary" pendingLabel="Assigning…">
        Assign
      </SubmitButton>
    </form>
  );
}
