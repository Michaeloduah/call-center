"use server";

import { redirect } from "next/navigation";

import { settingsApi } from "@/lib/api/settings";
import { type FormState, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";

export async function updateTenantSettingsAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const v = values(formData, ["offer_timeout_seconds", "max_queue_wait_seconds", "consent_text_override"]);
  try {
    await settingsApi.updateTenant({
      // Non-numeric input becomes NaN -> serialised as null -> rejected by the API with a field error.
      offer_timeout_seconds: Number.parseInt(v.offer_timeout_seconds, 10),
      max_queue_wait_seconds: Number.parseInt(v.max_queue_wait_seconds, 10),
      consent_text_override: v.consent_text_override === "" ? null : v.consent_text_override,
    });
  } catch (err) {
    return toFormState(err, v);
  }
  redirect("/admin/settings?notice=settings-saved");
}
