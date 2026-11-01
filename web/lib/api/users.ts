import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { InviteResult, Paginated, TenantRole, User, UserDetail } from "@/lib/types";

const path = (id: string) => `/v1/admin/users/${encodeURIComponent(id)}`;

const PICKER_PAGE_SIZE = 100;
const PICKER_MAX_USERS = 500; // matches the API's per-department assignment limit

export const usersApi = {
  list: (query: { q?: string; role?: string; status?: string; page?: number; per_page?: number }) =>
    apiFetch<Paginated<User>>("/v1/admin/users", { query }),
  get: (id: string) => apiFetch<UserDetail>(path(id)),
  create: (body: { full_name: string; email: string; role: TenantRole }) =>
    apiFetch<InviteResult>("/v1/admin/users", { method: "POST", body }),
  update: (id: string, body: { full_name: string; role: TenantRole }) =>
    apiFetch<User>(path(id), { method: "PUT", body }),
  disable: (id: string) => apiFetch<void>(`${path(id)}/disable`, { method: "POST" }),
  enable: (id: string) => apiFetch<void>(`${path(id)}/enable`, { method: "POST" }),
  resendInvite: (id: string) => apiFetch<InviteResult>(`${path(id)}/resend-invite`, { method: "POST" }),
  remove: (id: string) => apiFetch<void>(path(id), { method: "DELETE" }),
  /** Fetches every user matching the filter (bounded), for assignment pickers. */
  listAll: async (query: { role?: string; status?: string }): Promise<User[]> => {
    const users: User[] = [];
    for (let page = 1; users.length < PICKER_MAX_USERS; page++) {
      const res = await usersApi.list({ ...query, page, per_page: PICKER_PAGE_SIZE });
      users.push(...res.items);
      if (users.length >= res.total || res.items.length === 0) break;
    }
    return users;
  },
};
