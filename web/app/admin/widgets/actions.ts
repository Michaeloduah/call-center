"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { widgetsApi, type WidgetInput } from "@/lib/api/widgets";
import { type FormState, field, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { FieldMode } from "@/lib/types";

const echoFields = [
  "name", "department_id", "allowed_origins", "is_active", "button_label", "accent_color", "greeting",
  "form_enabled", "name_field", "phone_field", "email_field",
];

function widgetInput(formData: FormData): WidgetInput {
  return {
    name: field(formData, "name"),
    department_id: field(formData, "department_id"),
    allowed_origins: field(formData, "allowed_origins").split(/\r?\n/).map((o) => o.trim()).filter(Boolean),
    is_active: formData.get("is_active") === "on",
    button_label: field(formData, "button_label"),
    accent_color: field(formData, "accent_color"),
    greeting: field(formData, "greeting"),
    caller_form: {
      enabled: formData.get("form_enabled") === "on",
      // The API rejects anything other than hidden/optional/required.
      name: field(formData, "name_field") as FieldMode,
      phone: field(formData, "phone_field") as FieldMode,
      email: field(formData, "email_field") as FieldMode,
    },
  };
}

export async function createWidgetAction(_prev: FormState, formData: FormData): Promise<FormState> {
  let id: string;
  try {
    id = (await widgetsApi.create(widgetInput(formData))).id;
  } catch (err) {
    return toFormState(err, values(formData, echoFields));
  }
  revalidatePath("/admin/widgets");
  redirect(`/admin/widgets/${id}?notice=widget-created`);
}

export async function updateWidgetAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await widgetsApi.update(id, widgetInput(formData));
  } catch (err) {
    return toFormState(err, values(formData, echoFields));
  }
  revalidatePath("/admin/widgets");
  redirect(`/admin/widgets/${id}?notice=widget-updated`);
}

export async function deleteWidgetAction(id: string): Promise<void> {
  try {
    await widgetsApi.remove(id);
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) {
      redirect(`/admin/widgets/${id}?error=${encodeURIComponent(err.code)}`);
    }
    throw err;
  }
  revalidatePath("/admin/widgets");
  redirect("/admin/widgets?notice=widget-deleted");
}
