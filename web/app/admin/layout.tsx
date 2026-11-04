import { AppShell } from "@/components/layout/AppShell";
import { requireRole } from "@/lib/server/session";

const nav = [
  { href: "/admin", label: "Overview", exact: true },
  { href: "/admin/users", label: "Users" },
  { href: "/admin/departments", label: "Departments" },
  { href: "/admin/widgets", label: "Widgets" },
  { href: "/calls", label: "Call history" },
  { href: "/callbacks", label: "Callbacks" },
  { href: "/live", label: "Live board" },
  { href: "/agent", label: "Take calls" },
  { href: "/admin/settings", label: "Settings" },
];

export default async function AdminLayout({ children }: LayoutProps<"/admin">) {
  const user = await requireRole("tenant_admin");
  return (
    <AppShell user={user} nav={nav}>
      {children}
    </AppShell>
  );
}
