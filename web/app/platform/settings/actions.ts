"use server";

import { redirect } from "next/navigation";

import { settingsApi } from "@/lib/api/settings";
import { type FormState, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";

export async function updatePlatformSettingsAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const v = values(formData, ["default_consent_text"]);
  try {
    await settingsApi.updatePlatform(v.default_consent_text);
  } catch (err) {
    return toFormState(err, v);
  }
  redirect("/platform/settings?notice=settings-saved");
}
