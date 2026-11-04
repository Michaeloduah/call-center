import { AppShell } from "@/components/layout/AppShell";
import { requireRole } from "@/lib/server/session";

const nav = [
  { href: "/platform/tenants", label: "Tenants" },
  { href: "/platform/plans", label: "Plans" },
  { href: "/platform/recordings/deleted", label: "Deleted recordings" },
  { href: "/platform/audit-logs", label: "Audit log" },
  { href: "/platform/settings", label: "Settings" },
];

export default async function PlatformLayout({ children }: LayoutProps<"/platform">) {
  const user = await requireRole("platform_admin");
  return (
    <AppShell user={user} nav={nav}>
      {children}
    </AppShell>
  );
}
