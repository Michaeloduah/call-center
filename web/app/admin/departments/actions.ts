"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { departmentsApi, type DepartmentInput } from "@/lib/api/departments";
import { type FormState, field, values } from "@/lib/forms";
import { toFormState } from "@/lib/server/actions";
import { ApiError } from "@/lib/server/api";
import type { HoursRange } from "@/lib/types";

function departmentInput(formData: FormData): DepartmentInput {
  return {
    name: field(formData, "name"),
    description: field(formData, "description"),
    timezone: field(formData, "timezone"),
    is_active: formData.get("is_active") === "on",
  };
}

const echoFields = ["name", "description", "timezone", "is_active"];

export async function createDepartmentAction(_prev: FormState, formData: FormData): Promise<FormState> {
  let id: string;
  try {
    id = (await departmentsApi.create(departmentInput(formData))).id;
  } catch (err) {
    return toFormState(err, values(formData, echoFields));
  }
  revalidatePath("/admin/departments");
  redirect(`/admin/departments/${id}?notice=department-created`);
}

export async function updateDepartmentAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await departmentsApi.update(id, departmentInput(formData));
  } catch (err) {
    return toFormState(err, values(formData, echoFields));
  }
  revalidatePath("/admin/departments");
  redirect(`/admin/departments/${id}?notice=department-updated`);
}

export async function deleteDepartmentAction(id: string): Promise<void> {
  try {
    await departmentsApi.remove(id);
  } catch (err) {
    if (err instanceof ApiError && err.status !== 401) {
      redirect(`/admin/departments/${id}?error=${encodeURIComponent(err.code)}`);
    }
    throw err;
  }
  revalidatePath("/admin/departments");
  redirect("/admin/departments?notice=department-deleted");
}

function selectedUserIds(formData: FormData): string[] {
  return formData.getAll("user_ids").filter((v): v is string => typeof v === "string");
}

export async function setMembersAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await departmentsApi.setMembers(id, selectedUserIds(formData));
  } catch (err) {
    return toFormState(err);
  }
  revalidatePath(`/admin/departments/${id}`);
  redirect(`/admin/departments/${id}?notice=members-saved`);
}

export async function setSupervisorsAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  try {
    await departmentsApi.setSupervisors(id, selectedUserIds(formData));
  } catch (err) {
    return toFormState(err);
  }
  revalidatePath(`/admin/departments/${id}`);
  redirect(`/admin/departments/${id}?notice=supervisors-saved`);
}

/** The hours editor submits its rows as JSON in a hidden field; the API validates every range. */
export async function setBusinessHoursAction(id: string, _prev: FormState, formData: FormData): Promise<FormState> {
  let hours: HoursRange[];
  try {
    const parsed: unknown = JSON.parse(String(formData.get("hours") ?? "[]"));
    if (!Array.isArray(parsed)) throw new Error("not an array");
    hours = parsed.map((r) => ({ weekday: Number(r?.weekday), opens: String(r?.opens ?? ""), closes: String(r?.closes ?? "") }));
  } catch {
    return { status: "error", message: "The schedule could not be read. Please try again." };
  }
  try {
    await departmentsApi.setBusinessHours(id, hours);
  } catch (err) {
    return toFormState(err, { hours: JSON.stringify(hours) });
  }
  revalidatePath(`/admin/departments/${id}`);
  redirect(`/admin/departments/${id}?notice=hours-saved`);
}
