"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { usersApi } from "@/lib/api/users";
import { type FormState, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { TenantRole } from "@/lib/types";

export async function createUserAction(_prev: FormState, formData: FormData): Promise<FormState> {
  const v = values(formData, ["full_name", "email", "role"]);
  let target: string;
  try {
    const res = await usersApi.create({ full_name: v.full_name, email: v.email, role: v.role as TenantRole });
    target = `/admin/users/${res.user.id}?notice=${res.invite_email_sent ? "user-invited" : "user-invited-no-email"}`;
  } catch (err) {
    return toFormState(err, v);
  }
  revalidatePath("/admin/users");
  redirect(target);
}

export async function updateUserAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  const v = values(formData, ["full_name", "role"]);
  try {
    await usersApi.update(id, { full_name: v.full_name, role: v.role as TenantRole });
  } catch (err) {
    return toFormState(err, v);
  }
  revalidatePath("/admin/users");
  redirect(`/admin/users/${id}?notice=user-updated`);
}

/**
 * Runs a one-click user action. `run` returns the notice key to show on success; API errors
 * (e.g. removing the last admin) are surfaced via ?error= on the user's page.
 */
async function userAction(id: string, run: () => Promise<string>, target = `/admin/users/${id}`) {
  let notice: string;
  try {
    notice = await run();
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) {
      redirect(`/admin/users/${id}?error=${encodeURIComponent(err.code)}`);
    }
    throw err;
  }
  revalidatePath("/admin/users");
  redirect(`${target}?notice=${notice}`);
}

export async function disableUserAction(id: string): Promise<void> {
  await userAction(id, async () => {
    await usersApi.disable(id);
    return "user-disabled";
  });
}

export async function enableUserAction(id: string): Promise<void> {
  await userAction(id, async () => {
    await usersApi.enable(id);
    return "user-enabled";
  });
}

export async function deleteUserAction(id: string): Promise<void> {
  await userAction(id, async () => {
    await usersApi.remove(id);
    return "user-deleted";
  }, "/admin/users");
}

export async function resendInviteAction(id: string): Promise<void> {
  await userAction(id, async () =>
    (await usersApi.resendInvite(id)).invite_email_sent ? "invite-resent" : "invite-resent-no-email",
  );
}
