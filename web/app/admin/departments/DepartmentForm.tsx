"use client";

import { useActionState } from "react";

import { createDepartmentAction, updateDepartmentAction } from "@/app/admin/departments/actions";
import { LinkButton } from "@/components/ui/Button";
import { FormError } from "@/components/ui/FormError";
import { SelectField, TextAreaField, TextField } from "@/components/ui/Field";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { initialFormState } from "@/lib/forms";
import type { Department } from "@/lib/types";

/** Create/edit form. Pass `department` to edit. */
export function DepartmentForm({
  department,
  timezones,
}: {
  department?: Department;
  timezones: { value: string; label: string }[];
}) {
  const serverAction = department ? updateDepartmentAction.bind(null, department.id) : createDepartmentAction;
  const [state, action] = useActionState(serverAction, initialFormState);
  const v = state.values;
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextField name="name" label="Name" required maxLength={80} defaultValue={v?.name ?? department?.name} error={state.fieldErrors?.name} />
      <TextAreaField
        name="description"
        label="Description"
        rows={3}
        maxLength={500}
        defaultValue={v?.description ?? department?.description}
        error={state.fieldErrors?.description}
      />
      <SelectField
        name="timezone"
        label="Timezone"
        options={timezones}
        defaultValue={v?.timezone ?? department?.timezone ?? "UTC"}
        hint="Business hours are interpreted in this timezone."
        error={state.fieldErrors?.timezone}
      />
      <label className="flex items-center gap-2 text-sm text-slate-800">
        <input
          type="checkbox"
          name="is_active"
          defaultChecked={v ? v.is_active === "on" : (department?.is_active ?? true)}
          className="h-4 w-4 rounded border-slate-300 text-indigo-600"
        />
        Active (inactive departments will not receive calls)
      </label>
      <div className="flex gap-3">
        <SubmitButton>{department ? "Save changes" : "Create department"}</SubmitButton>
        <LinkButton href={department ? `/admin/departments/${department.id}` : "/admin/departments"} variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}
