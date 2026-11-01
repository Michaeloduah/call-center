"use client";

import { useActionState } from "react";

import { updatePlatformSettingsAction } from "@/app/platform/settings/actions";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { TextAreaField } from "@/components/ui/Field";
import { initialFormState } from "@/lib/forms";
import type { PlatformSettings } from "@/lib/types";

export function PlatformSettingsForm({ settings }: { settings: PlatformSettings }) {
  const [state, action] = useActionState(updatePlatformSettingsAction, initialFormState);
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <TextAreaField
        name="default_consent_text"
        label="Default recording-consent text"
        rows={4}
        maxLength={2000}
        required
        hint="Shown to callers before every call unless a tenant sets its own text."
        defaultValue={state.values?.default_consent_text ?? settings.default_consent_text}
        error={state.fieldErrors?.default_consent_text}
      />
      <SubmitButton>Save settings</SubmitButton>
    </form>
  );
}
