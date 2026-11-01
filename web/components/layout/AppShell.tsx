import { logoutAction } from "@/app/actions";
import { NavLink } from "@/components/layout/NavLink";
import { roleLabels } from "@/lib/format";
import type { Me } from "@/lib/types";

export interface NavItem {
  href: string;
  label: string;
  exact?: boolean;
}

export function AppShell({ user, nav, children }: { user: Me; nav: NavItem[]; children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col md:flex-row">
      <aside className="flex w-full shrink-0 flex-col bg-slate-900 md:min-h-screen md:w-60">
        <div className="px-5 py-5">
          <p className="text-base font-semibold text-white">CallCenter</p>
          <p className="mt-0.5 truncate text-xs text-slate-400">{user.tenant?.name ?? "Platform administration"}</p>
        </div>
        <nav aria-label="Main" className="flex-1 space-y-1 px-3">
          {nav.map((item) => (
            <NavLink key={item.href} href={item.href} exact={item.exact}>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-slate-800 px-5 py-4">
          <p className="truncate text-sm font-medium text-white">{user.full_name}</p>
          <p className="truncate text-xs text-slate-400">
            {roleLabels[user.role]} · {user.email}
          </p>
          <form action={logoutAction} className="mt-3">
            <button type="submit" className="text-sm text-slate-300 hover:text-white">
              Sign out
            </button>
          </form>
        </div>
      </aside>
      <main className="flex-1 px-4 py-8 sm:px-8">
        <div className="mx-auto max-w-5xl">{children}</div>
      </main>
    </div>
  );
}
