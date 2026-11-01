"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { tenantsApi } from "@/lib/api/tenants";
import { type FormState, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";

export async function createTenantAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const v = values(formData, ["name", "admin_name", "admin_email"]);
  let target: string;
  try {
    const res = await tenantsApi.create({ name: v.name, admin_name: v.admin_name, admin_email: v.admin_email });
    target = `/platform/tenants/${res.tenant.id}?notice=${res.invite_email_sent ? "tenant-created" : "tenant-created-no-email"}`;
  } catch (err) {
    return toFormState(err, v);
  }
  revalidatePath("/platform/tenants");
  redirect(target);
}

export async function updateTenantAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  const v = values(formData, ["name", "slug"]);
  try {
    await tenantsApi.update(id, { name: v.name, slug: v.slug });
  } catch (err) {
    return toFormState(err, v);
  }
  revalidatePath("/platform/tenants");
  redirect(`/platform/tenants/${id}?notice=tenant-updated`);
}

/** Runs a one-click tenant action; API errors (e.g. "suspend first") are surfaced via ?error=. */
async function tenantAction(id: string, run: () => Promise<string>, target = `/platform/tenants/${id}`) {
  let notice: string;
  try {
    notice = await run();
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) {
      redirect(`/platform/tenants/${id}?error=${encodeURIComponent(err.code)}`);
    }
    throw err;
  }
  revalidatePath("/platform/tenants");
  redirect(`${target}?notice=${notice}`);
}

export async function suspendTenantAction(id: string): Promise<void> {
  await tenantAction(id, async () => {
    await tenantsApi.suspend(id);
    return "tenant-suspended";
  });
}

export async function activateTenantAction(id: string): Promise<void> {
  await tenantAction(id, async () => {
    await tenantsApi.activate(id);
    return "tenant-activated";
  });
}

export async function deleteTenantAction(id: string): Promise<void> {
  await tenantAction(id, async () => {
    await tenantsApi.remove(id);
    return "tenant-deleted";
  }, "/platform/tenants");
}
