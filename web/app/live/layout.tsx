import { AppShell } from "@/components/layout/AppShell";
import { workspaceNav } from "@/lib/navigation";
import { requireRole } from "@/lib/server/session";

export default async function LiveLayout({ children }: LayoutProps<"/live">) {
  const user = await requireRole("supervisor", "tenant_admin");
  return (
    <AppShell user={user} nav={workspaceNav(user.role)}>
      {children}
    </AppShell>
  );
}
