import "server-only";

import { cookies } from "next/headers";

import { SESSION_COOKIE } from "@/lib/server/api";

/**
 * Relays a Server-Sent Events stream from the Go API to the browser. The browser only ever talks
 * to this Next.js server: the session cookie is turned into a bearer token here, so the API
 * needs no CORS, no cookies and no public exposure for real-time updates.
 */
export async function relayEventStream(request: Request, apiPath: string): Promise<Response> {
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  if (!token) return Response.json({ error: { code: "unauthenticated", message: "Please sign in." } }, { status: 401 });

  let upstream: Response;
  try {
    upstream = await fetch(new URL(apiPath, process.env.API_BASE_URL), {
      headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" },
      cache: "no-store",
      signal: request.signal, // closing the browser tab closes the upstream stream
    });
  } catch {
    return Response.json({ error: { code: "api_unreachable", message: "The service is temporarily unavailable." } }, { status: 503 });
  }
  if (!upstream.ok || !upstream.body) {
    // Pass the API's own error (401, 403, 429) straight through.
    return new Response(upstream.body, { status: upstream.status, headers: { "Content-Type": "application/json" } });
  }
  return new Response(upstream.body, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-store, no-transform",
      "X-Accel-Buffering": "no",
    },
  });
}
