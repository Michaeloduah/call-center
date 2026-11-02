import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { CallerForm, Paginated, Widget } from "@/lib/types";

const path = (id: string) => `/v1/admin/widgets/${encodeURIComponent(id)}`;

export interface WidgetInput {
  name: string;
  department_id: string;
  allowed_origins: string[];
  is_active: boolean;
  button_label: string;
  accent_color: string;
  greeting: string;
  caller_form: CallerForm;
}

export const widgetsApi = {
  list: (query: { q?: string; page?: number }) => apiFetch<Paginated<Widget>>("/v1/admin/widgets", { query }),
  get: (id: string) => apiFetch<Widget>(path(id)),
  create: (body: WidgetInput) => apiFetch<Widget>("/v1/admin/widgets", { method: "POST", body }),
  update: (id: string, body: WidgetInput) => apiFetch<Widget>(path(id), { method: "PUT", body }),
  remove: (id: string) => apiFetch<void>(path(id), { method: "DELETE" }),
};
