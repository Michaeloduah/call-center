import type { Metadata } from "next";

import { LiveBoard } from "@/app/live/LiveBoard";
import { PageHeader } from "@/components/ui/PageHeader";
import { liveApi } from "@/lib/api/agent";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Live board · CallCenter" };

export default async function LivePage() {
  await requireRole("supervisor", "tenant_admin");
  const board = await liveApi.board();
  return (
    <>
      <PageHeader title="Live board" description="Queues, calls in progress and agent availability, updated as it happens." />
      <LiveBoard initial={board} />
    </>
  );
}
