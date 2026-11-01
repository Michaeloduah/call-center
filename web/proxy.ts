import { NextResponse, type NextRequest } from "next/server";

const SESSION_COOKIE = "cc_session";

/**
 * Optimistic gate for signed-in areas: requests without a session cookie are redirected to the
 * login page before rendering. The real check (session validity and role) happens on the server
 * in each layout via requireRole, and again in the Go API.
 */
export function proxy(request: NextRequest) {
  if (request.cookies.has(SESSION_COOKIE)) return NextResponse.next();
  const login = new URL("/login", request.url);
  login.searchParams.set("next", request.nextUrl.pathname + request.nextUrl.search);
  return NextResponse.redirect(login);
}

export const config = {
  matcher: ["/platform/:path*", "/admin/:path*", "/home/:path*"],
};
