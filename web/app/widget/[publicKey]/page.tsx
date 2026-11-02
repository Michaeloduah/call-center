import type { Metadata } from "next";

import { CallWidget } from "@/app/widget/[publicKey]/CallWidget";
import { publicApi } from "@/lib/api/public";
import { orNotFound } from "@/lib/server/api";

export const metadata: Metadata = { title: "Call us", robots: { index: false } };

/**
 * The page loaded inside the widget iframe on a tenant's website. Which sites may embed it is
 * enforced by the CSP frame-ancestors header set in proxy.ts.
 */
export default async function WidgetPage({ params }: PageProps<"/widget/[publicKey]">) {
  const { publicKey } = await params;
  const config = await orNotFound(publicApi.getWidget(publicKey));
  return <CallWidget config={config} />;
}
