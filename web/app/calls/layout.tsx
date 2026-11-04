import { AppShell } from "@/components/layout/AppShell";
import { workspaceNav } from "@/lib/navigation";
import { requireRole } from "@/lib/server/session";

export default async function CallsLayout({ children }: LayoutProps<"/calls">) {
  const user = await requireRole("agent", "supervisor", "tenant_admin");
  return (
    <AppShell user={user} nav={workspaceNav(user.role)}>
      {children}
    </AppShell>
  );
}
