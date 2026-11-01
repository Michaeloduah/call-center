import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { Paginated, Tenant, TenantCreateResult, TenantDetail, TenantSummary } from "@/lib/types";

export const tenantsApi = {
  list: (query: { q?: string; status?: string; page?: number }) =>
    apiFetch<Paginated<TenantSummary>>("/v1/platform/tenants", { query }),
  get: (id: string) => apiFetch<TenantDetail>(`/v1/platform/tenants/${encodeURIComponent(id)}`),
  create: (body: { name: string; admin_name: string; admin_email: string }) =>
    apiFetch<TenantCreateResult>("/v1/platform/tenants", { method: "POST", body }),
  update: (id: string, body: { name: string; slug: string }) =>
    apiFetch<Tenant>(`/v1/platform/tenants/${encodeURIComponent(id)}`, { method: "PUT", body }),
  suspend: (id: string) => apiFetch<void>(`/v1/platform/tenants/${encodeURIComponent(id)}/suspend`, { method: "POST" }),
  activate: (id: string) => apiFetch<void>(`/v1/platform/tenants/${encodeURIComponent(id)}/activate`, { method: "POST" }),
  remove: (id: string) => apiFetch<void>(`/v1/platform/tenants/${encodeURIComponent(id)}`, { method: "DELETE" }),
};
