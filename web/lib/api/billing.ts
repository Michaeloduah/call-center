import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { BillingOverview, CapPolicy, Plan, PlanInterval, PlatformTenantBilling } from "@/lib/types";

const planPath = (id: string) => `/v1/platform/plans/${encodeURIComponent(id)}`;
const pricePath = (id: string) => `/v1/platform/prices/${encodeURIComponent(id)}`;

export interface PlanInput {
  code: string;
  name: string;
  description: string;
  is_public: boolean;
  is_active: boolean;
  max_agents: number | null;
  max_call_minutes_per_month: number | null;
  max_storage_bytes: number | null;
  max_departments: number | null;
  max_widgets: number | null;
}

export interface PriceInput {
  currency: string;
  interval: PlanInterval;
  amount_minor: number;
  overage_per_minute_minor: number;
  overage_per_gb_minor: number;
  is_active: boolean;
}

export const plansApi = {
  list: () => apiFetch<Plan[]>("/v1/platform/plans"),
  get: (id: string) => apiFetch<Plan>(planPath(id)),
  create: (body: PlanInput) => apiFetch<Plan>("/v1/platform/plans", { method: "POST", body }),
  update: (id: string, body: PlanInput) => apiFetch<Plan>(planPath(id), { method: "PUT", body }),
  remove: (id: string) => apiFetch<void>(planPath(id), { method: "DELETE" }),
  addPrice: (planId: string, body: PriceInput) => apiFetch<Plan>(`${planPath(planId)}/prices`, { method: "POST", body }),
  updatePrice: (id: string, body: { is_active: boolean; overage_per_minute_minor: number; overage_per_gb_minor: number }) =>
    apiFetch<void>(pricePath(id), { method: "PUT", body }),
  removePrice: (id: string) => apiFetch<void>(pricePath(id), { method: "DELETE" }),
  syncPrice: (id: string) => apiFetch<void>(`${pricePath(id)}/sync`, { method: "POST" }),
  tenantBilling: (tenantId: string) => apiFetch<PlatformTenantBilling>(`/v1/platform/tenants/${encodeURIComponent(tenantId)}/billing`),
};

export const billingApi = {
  overview: () => apiFetch<BillingOverview>("/v1/admin/billing"),
  setPolicies: (minutes: CapPolicy, storage: CapPolicy) =>
    apiFetch<void>("/v1/admin/billing/policies", { method: "PUT", body: { minutes_cap_policy: minutes, storage_cap_policy: storage } }),
  checkout: (priceId: string) => apiFetch<{ redirect_url: string }>("/v1/admin/billing/checkout", { method: "POST", body: { plan_price_id: priceId } }),
  cancel: () => apiFetch<void>("/v1/admin/billing/cancel", { method: "POST" }),
};
