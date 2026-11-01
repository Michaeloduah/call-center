import { AppShell } from "@/components/layout/AppShell";
import { requireRole } from "@/lib/server/session";

const nav = [{ href: "/home", label: "Home", exact: true }];

export default async function HomeLayout({ children }: LayoutProps<"/home">) {
  const user = await requireRole("agent", "supervisor");
  return (
    <AppShell user={user} nav={nav}>
      {children}
    </AppShell>
  );
}
