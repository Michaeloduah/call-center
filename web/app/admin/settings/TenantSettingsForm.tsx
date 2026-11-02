"use client";

import { useActionState } from "react";

import { updateTenantSettingsAction } from "@/app/admin/settings/actions";
import { FormError } from "@/components/ui/FormError";
import { TextAreaField, TextField } from "@/components/ui/Field";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { initialFormState } from "@/lib/forms";
import type { TenantSettings } from "@/lib/types";

export function TenantSettingsForm({ settings }: { settings: TenantSettings }) {
  const [state, action] = useActionState(updateTenantSettingsAction, initialFormState);
  const v = state.values;
  return (
    <form action={action} className="space-y-6" noValidate>
      <FormError state={state} />
      <fieldset className="space-y-5">
        <legend className="text-sm font-semibold text-slate-900">Call routing</legend>
        <TextField
          name="offer_timeout_seconds"
          type="number"
          min={5}
          max={120}
          label="Ring time per agent (seconds)"
          hint="How long an agent has to accept a call before it moves to the next agent (5–120)."
          defaultValue={v?.offer_timeout_seconds ?? settings.offer_timeout_seconds}
          error={state.fieldErrors?.offer_timeout_seconds}
        />
        <TextField
          name="max_queue_wait_seconds"
          type="number"
          min={30}
          max={3600}
          label="Maximum wait in queue (seconds)"
          hint="After this, the caller is offered a callback form instead (30–3600)."
          defaultValue={v?.max_queue_wait_seconds ?? settings.max_queue_wait_seconds}
          error={state.fieldErrors?.max_queue_wait_seconds}
        />
      </fieldset>
      <fieldset className="space-y-3">
        <legend className="text-sm font-semibold text-slate-900">Recording consent</legend>
        <TextAreaField
          name="consent_text_override"
          label="Custom consent text"
          rows={4}
          maxLength={2000}
          hint="Leave empty to use the platform default shown below. Callers must accept this text before calling."
          defaultValue={v?.consent_text_override ?? settings.consent_text_override ?? ""}
          error={state.fieldErrors?.consent_text_override}
        />
        <div className="rounded-md bg-slate-50 p-3 text-sm text-slate-600 ring-1 ring-slate-200">
          <p className="text-xs font-medium uppercase tracking-wide text-slate-500">Platform default</p>
          <p className="mt-1">{settings.default_consent_text}</p>
        </div>
      </fieldset>
      <SubmitButton>Save settings</SubmitButton>
    </form>
  );
}
