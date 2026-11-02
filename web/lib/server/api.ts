import "server-only";

import { cookies, headers } from "next/headers";
import { notFound } from "next/navigation";

export const SESSION_COOKIE = "cc_session";

const API_BASE_URL = process.env.API_BASE_URL;

/** Error returned by the Go API, with optional per-field validation messages. */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly fields: Record<string, string> = {},
  ) {
    super(message);
    this.name = "ApiError";
  }
}

type Query = Record<string, string | number | boolean | undefined | null>;

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
  query?: Query;
  /** Send the session cookie as a bearer token. Defaults to true. */
  auth?: boolean;
  /** Extra request headers (e.g. the caller token on public call endpoints). */
  headers?: Record<string, string>;
}

function buildUrl(path: string, query?: Query): string {
  if (!API_BASE_URL) {
    throw new Error("API_BASE_URL is not configured");
  }
  const url = new URL(path, API_BASE_URL);
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== "") {
      url.searchParams.set(key, String(value));
    }
  }
  return url.toString();
}

/**
 * Calls the Go API from the Next.js server. The browser never talks to the API directly:
 * the session token lives in an HttpOnly cookie and is forwarded as a bearer token, and the
 * client IP / user agent are forwarded for auditing and rate limiting.
 */
export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, query, auth = true, headers: extraHeaders } = options;
  const incoming = await headers();
  const outgoing = new Headers({ Accept: "application/json" });

  const forwardedFor = incoming.get("x-forwarded-for");
  if (forwardedFor) outgoing.set("X-Forwarded-For", forwardedFor);
  const userAgent = incoming.get("user-agent");
  if (userAgent) outgoing.set("X-Forwarded-User-Agent", userAgent);

  if (auth) {
    const token = (await cookies()).get(SESSION_COOKIE)?.value;
    if (token) outgoing.set("Authorization", `Bearer ${token}`);
  }
  if (body !== undefined) outgoing.set("Content-Type", "application/json");
  for (const [name, value] of Object.entries(extraHeaders ?? {})) outgoing.set(name, value);

  let res: Response;
  try {
    res = await fetch(buildUrl(path, query), {
      method,
      headers: outgoing,
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    });
  } catch {
    throw new ApiError(503, "api_unreachable", "The service is temporarily unavailable. Please try again.", {});
  }

  if (res.status === 204) return undefined as T;

  const payload = await res.json().catch(() => null);
  if (!res.ok) {
    const err = payload?.error;
    throw new ApiError(
      res.status,
      err?.code ?? "unknown_error",
      err?.message ?? "Something went wrong. Please try again.",
      err?.fields ?? {},
    );
  }
  return payload as T;
}

/** Awaits an API call for a detail page, rendering the route's 404 page when the resource is missing. */
export async function orNotFound<T>(request: Promise<T>): Promise<T> {
  try {
    return await request;
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) notFound();
    throw err;
  }
}
