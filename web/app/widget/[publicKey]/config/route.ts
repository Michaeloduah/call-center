import { publicApi } from "@/lib/api/public";
import { ApiError } from "@/lib/server/api";

const CONFIG_MAX_AGE_SECONDS = 60;

/**
 * Minimal, public configuration for widget.js on the tenant's site: just enough to draw the
 * button. Sites that are not in the widget's allowed list get a 404, so the button only appears
 * where the call window may actually be embedded (the iframe is separately protected by CSP
 * frame-ancestors in proxy.ts, which browsers enforce).
 */
export async function GET(req: Request, ctx: RouteContext<"/widget/[publicKey]/config">) {
  const { publicKey } = await ctx.params;
  const origin = req.headers.get("origin");
  const headers: Record<string, string> = {
    "Cache-Control": `public, max-age=${CONFIG_MAX_AGE_SECONDS}`,
    Vary: "Origin",
  };
  const notFound = () => Response.json({ error: "not_found" }, { status: 404, headers });

  let cfg;
  try {
    cfg = await publicApi.getWidget(publicKey);
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return notFound();
    throw err;
  }
  if (!origin || !cfg.frame_ancestors.includes(origin)) return notFound();

  headers["Access-Control-Allow-Origin"] = origin;
  return Response.json({ button_label: cfg.button_label, accent_color: cfg.accent_color }, { headers });
}
