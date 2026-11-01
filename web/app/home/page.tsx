import type { Metadata } from "next";

import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";
import { roleLabels } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Home · CallCenter" };

/**
 * Landing page for agents and supervisors. The agent console and the supervisor live board are
 * delivered in milestone M3; until then this confirms the account is set up correctly.
 */
export default async function HomePage() {
  const user = await requireRole("agent", "supervisor");
  return (
    <>
      <PageHeader title={`Welcome, ${user.full_name}`} description={`${roleLabels[user.role]} at ${user.tenant?.name ?? ""}`} />
      <Card>
        <p className="text-sm text-slate-700">
          Your account is active. The {user.role === "agent" ? "agent console for taking calls" : "live board for your departments"} will
          appear here once calling is enabled for your organisation.
        </p>
      </Card>
    </>
  );
}
