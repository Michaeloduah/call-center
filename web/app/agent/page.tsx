import type { Metadata } from "next";

import { AgentConsole } from "@/app/agent/AgentConsole";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { agentApi } from "@/lib/api/agent";
import { ApiError } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Take calls · CallCenter" };

export default async function AgentPage() {
  await requireRole("agent", "supervisor", "tenant_admin");

  let session;
  try {
    session = await agentApi.session();
  } catch (err) {
    // Users who are not assigned to a department cannot take calls.
    if (err instanceof ApiError && err.status === 403) {
      return (
        <>
          <PageHeader title="Take calls" />
          <EmptyState
            title="You're not assigned to a department"
            description="Ask an administrator to add you to a department, then reload this page."
          />
        </>
      );
    }
    throw err;
  }

  return (
    <>
      <PageHeader title="Take calls" description="Set yourself available to receive calls from your departments." />
      <AgentConsole initial={session} />
    </>
  );
}
