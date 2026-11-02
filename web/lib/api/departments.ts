import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { Department, DepartmentDetail, DepartmentSummary, HoursRange, Paginated } from "@/lib/types";

const path = (id: string) => `/v1/admin/departments/${encodeURIComponent(id)}`;
const SELECT_MAX = 500;

export interface DepartmentInput {
  name: string;
  description: string;
  timezone: string;
  is_active: boolean;
}

export const departmentsApi = {
  list: (query: { q?: string; active?: string; page?: number }) =>
    apiFetch<Paginated<DepartmentSummary>>("/v1/admin/departments", { query }),
  get: (id: string) => apiFetch<DepartmentDetail>(path(id)),
  /** Every department (bounded), for selects. */
  listAll: async (): Promise<DepartmentSummary[]> => {
    const out: DepartmentSummary[] = [];
    for (let page = 1; out.length < SELECT_MAX; page++) {
      const res = await apiFetch<Paginated<DepartmentSummary>>("/v1/admin/departments", { query: { page, per_page: 100 } });
      out.push(...res.items);
      if (out.length >= res.total || res.items.length === 0) break;
    }
    return out;
  },
  create: (body: DepartmentInput) => apiFetch<Department>("/v1/admin/departments", { method: "POST", body }),
  update: (id: string, body: DepartmentInput) => apiFetch<Department>(path(id), { method: "PUT", body }),
  remove: (id: string) => apiFetch<void>(path(id), { method: "DELETE" }),
  setMembers: (id: string, userIds: string[]) =>
    apiFetch<DepartmentDetail>(`${path(id)}/members`, { method: "PUT", body: { user_ids: userIds } }),
  setSupervisors: (id: string, userIds: string[]) =>
    apiFetch<DepartmentDetail>(`${path(id)}/supervisors`, { method: "PUT", body: { user_ids: userIds } }),
  setBusinessHours: (id: string, hours: HoursRange[]) =>
    apiFetch<DepartmentDetail>(`${path(id)}/business-hours`, { method: "PUT", body: { business_hours: hours } }),
};
