"use client";

import { useActionState } from "react";

import { createTenantAction, updateTenantAction } from "@/app/platform/tenants/actions";
import { LinkButton } from "@/components/ui/Button";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { TextField } from "@/components/ui/Field";
import { initialFormState } from "@/lib/forms";
import type { Tenant } from "@/lib/types";

export function CreateTenantForm() {
  const [state, action] = useActionState(createTenantAction, initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextField name="name" label="Business name" required defaultValue={state.values?.name} error={state.fieldErrors?.name} />
      <fieldset className="space-y-5 rounded-md border border-slate-200 p-4">
        <legend className="px-1 text-sm font-medium text-slate-700">First admin</legend>
        <TextField
          name="admin_name"
          label="Full name"
          required
          defaultValue={state.values?.admin_name}
          error={state.fieldErrors?.admin_name}
        />
        <TextField
          name="admin_email"
          type="email"
          label="Email"
          required
          hint="They will receive an invitation to set their password."
          defaultValue={state.values?.admin_email}
          error={state.fieldErrors?.admin_email ?? state.fieldErrors?.email}
        />
      </fieldset>
      <div className="flex gap-3">
        <SubmitButton pendingLabel="Creating…">Create tenant</SubmitButton>
        <LinkButton href="/platform/tenants" variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}

export function EditTenantForm({ tenant }: { tenant: Tenant }) {
  const [state, action] = useActionState(updateTenantAction.bind(null, tenant.id), initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextField
        name="name"
        label="Business name"
        required
        defaultValue={state.values?.name ?? tenant.name}
        error={state.fieldErrors?.name}
      />
      <TextField
        name="slug"
        label="Slug"
        required
        hint="Lowercase letters, numbers and hyphens."
        defaultValue={state.values?.slug ?? tenant.slug}
        error={state.fieldErrors?.slug}
      />
      <div className="flex gap-3">
        <SubmitButton>Save changes</SubmitButton>
        <LinkButton href={`/platform/tenants/${tenant.id}`} variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}
