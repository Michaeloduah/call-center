import type { NavItem } from "@/components/layout/AppShell";
import type { Role } from "@/lib/types";

/** Sidebar for the call-handling areas (/agent, /live), which every tenant role can reach. */
export function workspaceNav(role: Role): NavItem[] {
  const items: NavItem[] = [
    { href: "/agent", label: "Take calls" },
    { href: "/callbacks", label: "Callbacks" },
    { href: "/calls", label: "Call history" },
  ];
  if (role === "supervisor" || role === "tenant_admin") items.push({ href: "/live", label: "Live board" });
  if (role === "tenant_admin") items.push({ href: "/admin", label: "Administration" });
  return items;
}
