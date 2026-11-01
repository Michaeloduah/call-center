import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { AuditLogEntry, Paginated, PlatformSettings, TenantSettings } from "@/lib/types";

export const settingsApi = {
  getPlatform: () => apiFetch<PlatformSettings>("/v1/platform/settings"),
  updatePlatform: (defaultConsentText: string) =>
    apiFetch<PlatformSettings>("/v1/platform/settings", {
      method: "PUT",
      body: { default_consent_text: defaultConsentText },
    }),
  getTenant: () => apiFetch<TenantSettings>("/v1/admin/settings"),
  updateTenant: (body: {
    offer_timeout_seconds: number;
    max_queue_wait_seconds: number;
    consent_text_override: string | null;
  }) => apiFetch<TenantSettings>("/v1/admin/settings", { method: "PUT", body }),
};

export const auditApi = {
  list: (query: { tenant_id?: string; action?: string; page?: number }) =>
    apiFetch<Paginated<AuditLogEntry>>("/v1/platform/audit-logs", { query }),
};
