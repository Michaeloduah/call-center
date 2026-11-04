"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { type PlanInput, type PriceInput, plansApi } from "@/lib/api/billing";
import { type FormState, field, values } from "@/lib/forms";
import { toMinor } from "@/lib/format";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { PlanInterval } from "@/lib/types";

const GB = 1024 ** 3;
const planFields = ["code", "name", "description", "is_public", "is_active", "max_agents", "max_call_minutes", "max_storage_gb", "max_departments", "max_widgets"];

/** Empty means unlimited (null). Anything else must be a whole number, or the field is reported. */
function limit(formData: FormData, name: string, errors: Record<string, string>, scale = 1): number | null {
  const raw = field(formData, name);
  if (raw === "") return null;
  const n = Number(raw);
  if (!Number.isFinite(n) || n < 0 || (scale === 1 && !Number.isInteger(n))) {
    errors[name] = scale === 1 ? "Enter a whole number, or leave empty for unlimited." : "Enter a number, or leave empty for unlimited.";
    return null;
  }
  return Math.round(n * scale);
}

function planInput(formData: FormData): { input: PlanInput; errors: Record<string, string> } {
  const errors: Record<string, string> = {};
  return {
    errors,
    input: {
      code: field(formData, "code"),
      name: field(formData, "name"),
      description: field(formData, "description"),
      is_public: formData.get("is_public") === "on",
      is_active: formData.get("is_active") === "on",
      max_agents: limit(formData, "max_agents", errors),
      max_call_minutes_per_month: limit(formData, "max_call_minutes", errors),
      max_storage_bytes: limit(formData, "max_storage_gb", errors, GB),
      max_departments: limit(formData, "max_departments", errors),
      max_widgets: limit(formData, "max_widgets", errors),
    },
  };
}

function invalid(formData: FormData, errors: Record<string, string>): FormState {
  return { status: "error", message: "Please correct the highlighted fields.", fieldErrors: errors, values: values(formData, planFields) };
}

export async function createPlanAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const { input, errors } = planInput(formData);
  if (Object.keys(errors).length > 0) return invalid(formData, errors);
  let id: string;
  try {
    id = (await plansApi.create(input)).id;
  } catch (err) {
    return toFormState(err, values(formData, planFields));
  }
  revalidatePath("/platform/plans");
  redirect(`/platform/plans/${id}?notice=plan-created`);
}

export async function updatePlanAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  const { input, errors } = planInput(formData);
  if (Object.keys(errors).length > 0) return invalid(formData, errors);
  try {
    await plansApi.update(id, input);
  } catch (err) {
    return toFormState(err, values(formData, planFields));
  }
  revalidatePath("/platform/plans");
  redirect(`/platform/plans/${id}?notice=plan-updated`);
}

async function oneClick(planId: string, notice: string, run: () => Promise<unknown>, to = `/platform/plans/${planId}`): Promise<never> {
  try {
    await run();
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) redirect(`${to}?error=${encodeURIComponent(err.code)}`);
    throw err;
  }
  revalidatePath("/platform/plans");
  redirect(`${to}?notice=${notice}`);
}

export async function deletePlanAction(id: string): Promise<void> {
  await oneClick(id, "plan-deleted", () => plansApi.remove(id), "/platform/plans");
}

export async function syncPriceAction(planId: string, priceId: string): Promise<void> {
  await oneClick(planId, "price-synced", () => plansApi.syncPrice(priceId));
}

export async function deletePriceAction(planId: string, priceId: string): Promise<void> {
  await oneClick(planId, "price-deleted", () => plansApi.removePrice(priceId));
}

export async function setPriceActiveAction(planId: string, priceId: string, active: boolean, perMinute: number, perGb: number): Promise<void> {
  await oneClick(planId, "price-updated", () =>
    plansApi.updatePrice(priceId, { is_active: active, overage_per_minute_minor: perMinute, overage_per_gb_minor: perGb }),
  );
}

export async function createPriceAction(planId: string, _prev: FormState, formData: FormData): Promise<FormState> {
  const currency = field(formData, "currency").toUpperCase();
  const echo = values(formData, ["currency", "interval", "amount", "overage_minute", "overage_gb"]);
  const errors: Record<string, string> = {};
  let amount = 0;
  let perMinute = 0;
  let perGb = 0;
  try {
    const minor = (name: string): number => {
      const n = toMinor(field(formData, name), currency);
      if (n === null) errors[name] = "Enter an amount such as 49.00.";
      return n ?? 0;
    };
    amount = minor("amount");
    perMinute = minor("overage_minute");
    perGb = minor("overage_gb");
  } catch {
    errors.currency = "Use a three-letter currency code such as NGN or USD."; // Intl rejects unknown codes
  }
  if (Object.keys(errors).length > 0) return { status: "error", message: "Please correct the highlighted fields.", fieldErrors: errors, values: echo };

  const input: PriceInput = {
    currency, interval: field(formData, "interval") as PlanInterval, amount_minor: amount,
    overage_per_minute_minor: perMinute, overage_per_gb_minor: perGb, is_active: true,
  };
  try {
    await plansApi.addPrice(planId, input);
  } catch (err) {
    const state = toFormState(err, echo);
    // The API names fields in minor units; show its message against the form's field.
    if (state.fieldErrors?.amount_minor) state.fieldErrors.amount = state.fieldErrors.amount_minor;
    return state;
  }
  revalidatePath(`/platform/plans/${planId}`);
  redirect(`/platform/plans/${planId}?notice=price-added`);
}
