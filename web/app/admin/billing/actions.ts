"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { billingApi } from "@/lib/api/billing";
import { type FormState, field } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { CapPolicy } from "@/lib/types";

const policy = (value: string): CapPolicy => (value === "overage" ? "overage" : "block");

export async function savePoliciesAction(_prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await billingApi.setPolicies(policy(field(formData, "minutes_cap_policy")), policy(field(formData, "storage_cap_policy")));
  } catch (err) {
    return toFormState(err, { minutes_cap_policy: field(formData, "minutes_cap_policy"), storage_cap_policy: field(formData, "storage_cap_policy") });
  }
  revalidatePath("/admin/billing");
  redirect("/admin/billing?notice=policies-saved");
}

/** Starts a hosted checkout and sends the browser to the payment provider's page. */
export async function checkoutAction(priceId: string): Promise<void> {
  let url: string;
  try {
    url = (await billingApi.checkout(priceId)).redirect_url;
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) redirect(`/admin/billing?error=${encodeURIComponent(err.code)}`);
    throw err;
  }
  redirect(url);
}

export async function cancelSubscriptionAction(): Promise<void> {
  try {
    await billingApi.cancel();
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) redirect(`/admin/billing?error=${encodeURIComponent(err.code)}`);
    throw err;
  }
  revalidatePath("/admin/billing");
  redirect("/admin/billing?notice=subscription-cancelled");
}
