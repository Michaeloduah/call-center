import { NextResponse, type NextRequest } from "next/server";

const SESSION_COOKIE = "cc_session";
const WIDGET_PAGE = /^\/widget\/(wk_[A-Za-z0-9_-]{22})$/;

/**
 * 1. Signed-in areas: requests without a session cookie are redirected to the login page before
 *    rendering. The real check (session validity and role) happens on the server in each page via
 *    requireRole, and again in the Go API.
 * 2. Widget iframe: only the tenant's allowed websites may embed it. Browsers enforce this through
 *    the CSP frame-ancestors directive, which must be sent as a response header.
 */
export async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  if (pathname.startsWith("/widget/")) return widgetFrameAncestors(request, pathname);

  if (request.cookies.has(SESSION_COOKIE)) return NextResponse.next();
  const login = new URL("/login", request.url);
  login.searchParams.set("next", pathname + request.nextUrl.search);
  return NextResponse.redirect(login);
}

async function widgetFrameAncestors(request: NextRequest, pathname: string) {
  const match = WIDGET_PAGE.exec(pathname);
  if (!match) return NextResponse.next(); // e.g. /widget/<key>/config, which is meant to be fetched cross-origin

  let ancestors = "'none'";
  try {
    const res = await fetch(new URL(`/v1/public/widgets/${match[1]}`, process.env.API_BASE_URL), { cache: "no-store" });
    if (res.ok) {
      const cfg: { frame_ancestors?: string[] } = await res.json();
      // Origins were validated by the API (scheme://host[:port] only), so they are safe to emit.
      if (cfg.frame_ancestors?.length) ancestors = cfg.frame_ancestors.join(" ");
    }
  } catch {
    // API unreachable: fail closed (no embedding allowed). The page itself will show an error.
  }
  const response = NextResponse.next();
  response.headers.set("Content-Security-Policy", `frame-ancestors ${ancestors}`);
  return response;
}

export const config = {
  matcher: ["/platform/:path*", "/admin/:path*", "/home/:path*", "/widget/:path*"],
};
