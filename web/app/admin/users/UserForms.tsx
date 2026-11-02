"use client";

import { useActionState } from "react";

import { createUserAction, updateUserAction } from "@/app/admin/users/actions";
import { LinkButton } from "@/components/ui/Button";
import { FormError } from "@/components/ui/FormError";
import { SelectField, TextField } from "@/components/ui/Field";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { initialFormState } from "@/lib/forms";
import type { User } from "@/lib/types";

const roleOptions = [
  { value: "agent", label: "Agent — answers calls" },
  { value: "supervisor", label: "Supervisor — monitors assigned departments" },
  { value: "tenant_admin", label: "Admin — manages the organisation" },
];

export function CreateUserForm() {
  const [state, action] = useActionState(createUserAction, initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextField name="full_name" label="Full name" required defaultValue={state.values?.full_name} error={state.fieldErrors?.full_name} />
      <TextField
        name="email"
        type="email"
        label="Email"
        required
        hint="They will receive an invitation to set their password."
        defaultValue={state.values?.email}
        error={state.fieldErrors?.email}
      />
      <SelectField name="role" label="Role" options={roleOptions} defaultValue={state.values?.role ?? "agent"} error={state.fieldErrors?.role} />
      <div className="flex gap-3">
        <SubmitButton pendingLabel="Inviting…">Send invitation</SubmitButton>
        <LinkButton href="/admin/users" variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}

export function EditUserForm({ user, isSelf }: { user: User; isSelf: boolean }) {
  const [state, action] = useActionState(updateUserAction.bind(null, user.id), initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextField
        name="full_name"
        label="Full name"
        required
        defaultValue={state.values?.full_name ?? user.full_name}
        error={state.fieldErrors?.full_name}
      />
      <TextField name="email" label="Email" value={user.email} disabled readOnly hint="Email addresses cannot be changed." />
      {isSelf ? (
        <>
          <input type="hidden" name="role" value={user.role} />
          <p className="text-sm text-slate-500">You cannot change your own role.</p>
        </>
      ) : (
        <SelectField
          name="role"
          label="Role"
          options={roleOptions}
          defaultValue={state.values?.role ?? user.role}
          error={state.fieldErrors?.role}
          hint="Changing a supervisor to another role removes their department assignments."
        />
      )}
      <div className="flex gap-3">
        <SubmitButton>Save changes</SubmitButton>
        <LinkButton href={`/admin/users/${user.id}`} variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}
