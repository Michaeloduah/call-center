"use client";

import { useActionState } from "react";

import { createWidgetAction, updateWidgetAction } from "@/app/admin/widgets/actions";
import { LinkButton } from "@/components/ui/Button";
import { inputBaseClass } from "@/components/ui/Field";
import { FormError } from "@/components/ui/FormError";
import { SelectField, TextAreaField, TextField } from "@/components/ui/Field";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { initialFormState } from "@/lib/forms";
import type { DepartmentSummary, Widget } from "@/lib/types";

const fieldModes = [
  { value: "hidden", label: "Hidden" },
  { value: "optional", label: "Optional" },
  { value: "required", label: "Required" },
];

/** Create/edit form. Pass `widget` to edit. */
export function WidgetForm({ widget, departments }: { widget?: Widget; departments: DepartmentSummary[] }) {
  const serverAction = widget ? updateWidgetAction.bind(null, widget.id) : createWidgetAction;
  const [state, action] = useActionState(serverAction, initialFormState);
  const v = state.values;
  const e = state.fieldErrors ?? {};
  const checked = (name: string, fallback: boolean) => (v ? v[name] === "on" : fallback);

  return (
    <form action={action} className="space-y-8" noValidate>
      <FormError state={state} />

      <fieldset className="space-y-5">
        <legend className="text-sm font-semibold text-slate-900">General</legend>
        <TextField name="name" label="Name" required maxLength={80} hint="For your reference, e.g. “Main website”."
          defaultValue={v?.name ?? widget?.name} error={e.name} />
        <SelectField
          name="department_id"
          label="Route calls to"
          options={[{ value: "", label: "Choose a department…" }, ...departments.map((d) => ({ value: d.id, label: d.is_active ? d.name : `${d.name} (inactive)` }))]}
          defaultValue={v?.department_id ?? widget?.department_id ?? ""}
          error={e.department_id}
        />
        <TextAreaField
          name="allowed_origins"
          label="Websites allowed to embed this widget"
          rows={3}
          hint="One per line, e.g. https://www.example.com. Plain http is only allowed for localhost."
          defaultValue={v?.allowed_origins ?? widget?.allowed_origins.join("\n")}
          error={e.allowed_origins}
        />
        <label className="flex items-center gap-2 text-sm text-slate-800">
          <input type="checkbox" name="is_active" defaultChecked={checked("is_active", widget?.is_active ?? true)}
            className="h-4 w-4 rounded border-slate-300 text-indigo-600" />
          Active (inactive widgets are hidden from your website)
        </label>
      </fieldset>

      <fieldset className="space-y-5">
        <legend className="text-sm font-semibold text-slate-900">Appearance</legend>
        <TextField name="button_label" label="Button label" required maxLength={40}
          defaultValue={v?.button_label ?? widget?.button_label ?? "Call us"} error={e.button_label} />
        <div className="space-y-1.5">
          <label htmlFor="accent_color" className="block text-sm font-medium text-slate-800">Accent colour</label>
          <input id="accent_color" name="accent_color" type="color"
            defaultValue={v?.accent_color ?? widget?.accent_color ?? "#4f46e5"}
            className={`${inputBaseClass} h-10 w-20 p-1`} aria-invalid={e.accent_color ? true : undefined} />
          {e.accent_color ? <p className="text-sm text-red-600">{e.accent_color}</p> : null}
        </div>
        <TextAreaField name="greeting" label="Greeting" rows={2} maxLength={200}
          hint="Shown at the top of the call window." defaultValue={v?.greeting ?? widget?.greeting} error={e.greeting} />
      </fieldset>

      <fieldset className="space-y-5">
        <legend className="text-sm font-semibold text-slate-900">Caller form</legend>
        <label className="flex items-center gap-2 text-sm text-slate-800">
          <input type="checkbox" name="form_enabled" defaultChecked={checked("form_enabled", widget?.caller_form.enabled ?? true)}
            className="h-4 w-4 rounded border-slate-300 text-indigo-600" />
          Ask callers for their details before the call
        </label>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <SelectField name="name_field" label="Name" options={fieldModes}
            defaultValue={v?.name_field ?? widget?.caller_form.name ?? "required"} error={e["caller_form.name"]} />
          <SelectField name="phone_field" label="Phone" options={fieldModes}
            defaultValue={v?.phone_field ?? widget?.caller_form.phone ?? "optional"} error={e["caller_form.phone"]} />
          <SelectField name="email_field" label="Email" options={fieldModes}
            defaultValue={v?.email_field ?? widget?.caller_form.email ?? "hidden"} error={e["caller_form.email"]} />
        </div>
        <p className="text-sm text-slate-500">
          Callers must always agree to the recording-consent text (set in Settings) before calling.
        </p>
      </fieldset>

      <div className="flex gap-3">
        <SubmitButton>{widget ? "Save changes" : "Create widget"}</SubmitButton>
        <LinkButton href={widget ? `/admin/widgets/${widget.id}` : "/admin/widgets"} variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}
