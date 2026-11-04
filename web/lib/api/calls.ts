import "server-only";

import { apiFetch } from "@/lib/server/api";
import type { AccessGrant, CallDetail, CallListItem, Callback, DeletedRecording, Paginated } from "@/lib/types";

const callPath = (id: string, suffix = "") => `/v1/calls/${encodeURIComponent(id)}${suffix}`;
const callbackPath = (id: string, suffix = "") => `/v1/callbacks/${encodeURIComponent(id)}${suffix}`;
const platformRecordingPath = (id: string, suffix: string) => `/v1/platform/recordings/${encodeURIComponent(id)}/${suffix}`;

export interface CallFilters {
  q?: string;
  status?: string;
  department_id?: string;
  from?: string;
  to?: string;
  page?: number;
}

export const callsApi = {
  list: (query: CallFilters) => apiFetch<Paginated<CallListItem>>("/v1/calls", { query: { ...query } }),
  get: (id: string) => apiFetch<CallDetail>(callPath(id)),
  recordingAccess: (id: string, download: boolean) =>
    apiFetch<AccessGrant>(callPath(id, "/recording/access"), { method: "POST", body: { download } }),
  deleteRecording: (id: string) => apiFetch<void>(callPath(id, "/recording"), { method: "DELETE" }),
};

export const callbacksApi = {
  list: (query: { status?: string; department_id?: string; page?: number }) =>
    apiFetch<Paginated<Callback>>("/v1/callbacks", { query: { ...query } }),
  get: (id: string) => apiFetch<Callback>(callbackPath(id)),
  claim: (id: string) => apiFetch<void>(callbackPath(id, "/claim"), { method: "POST" }),
  release: (id: string) => apiFetch<void>(callbackPath(id, "/release"), { method: "POST" }),
  complete: (id: string, notes: string) => apiFetch<void>(callbackPath(id, "/complete"), { method: "POST", body: { notes } }),
  cancel: (id: string) => apiFetch<void>(callbackPath(id, "/cancel"), { method: "POST" }),
  assign: (id: string, userId: string) =>
    apiFetch<void>(`/v1/admin/callbacks/${encodeURIComponent(id)}/assign`, { method: "POST", body: { user_id: userId } }),
};

export const platformRecordingsApi = {
  listDeleted: (query: { page?: number }) => apiFetch<Paginated<DeletedRecording>>("/v1/platform/recordings/deleted", { query }),
  access: (id: string, download: boolean) =>
    apiFetch<AccessGrant>(platformRecordingPath(id, "access"), { method: "POST", body: { download } }),
  restore: (id: string) => apiFetch<void>(platformRecordingPath(id, "restore"), { method: "POST" }),
};
