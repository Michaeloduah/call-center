import type { Metadata } from "next";
import Link from "next/link";

import { CopyButton } from "@/app/admin/widgets/CopyButton";
import { deleteWidgetAction } from "@/app/admin/widgets/actions";
import { ActionError } from "@/components/ui/ActionError";
import { Badge } from "@/components/ui/Badge";
import { LinkButton } from "@/components/ui/Button";
import { Card, DescriptionList } from "@/components/ui/Card";
import { ConfirmButton } from "@/components/ui/ConfirmButton";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { widgetsApi } from "@/lib/api/widgets";
import { formatDateTime } from "@/lib/format";
import { orNotFound } from "@/lib/server/api";
import { requireRole } from "@/lib/server/session";
import type { FieldMode } from "@/lib/types";

export const metadata: Metadata = { title: "Widget · CallCenter" };

const modeLabel: Record<FieldMode, string> = { hidden: "Hidden", optional: "Optional", required: "Required" };

export default async function WidgetDetailPage({ params, searchParams }: PageProps<"/admin/widgets/[id]">) {
  await requireRole("tenant_admin");
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  const w = await orNotFound(widgetsApi.get(id));
  const webUrl = process.env.WEB_PUBLIC_URL ?? "";
  const snippet = `<script src="${webUrl}/widget.js" data-widget-key="${w.public_key}" async></script>`;

  return (
    <>
      <PageHeader
        title={w.name}
        back={{ href: "/admin/widgets", label: "Widgets" }}
        actions={
          <>
            <LinkButton href={`/admin/widgets/${w.id}/edit`} variant="secondary">
              Edit
            </LinkButton>
            <ConfirmButton
              action={deleteWidgetAction.bind(null, w.id)}
              confirmMessage={`Delete the ${w.name} widget? It will stop working on your websites immediately. Call history is kept.`}
              variant="danger"
              pendingLabel="Deleting…"
            >
              Delete
            </ConfirmButton>
          </>
        }
      />
      <Notice value={sp.notice} />
      <ActionError value={sp.error} />
      <div className="space-y-6">
        <Card title="Embed code" actions={<CopyButton text={snippet} />}>
          <p className="mb-3 text-sm text-slate-600">
            Paste this just before the closing <code className="font-mono">&lt;/body&gt;</code> tag on each allowed website.
          </p>
          <pre className="overflow-x-auto rounded-md bg-slate-900 p-4 text-xs text-slate-100">
            <code>{snippet}</code>
          </pre>
          {!w.is_active ? (
            <p className="mt-3 text-sm text-amber-800">This widget is inactive, so the button will not appear on your website.</p>
          ) : null}
        </Card>
        <Card title="Details">
          <DescriptionList
            items={[
              { label: "Status", value: w.is_active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge> },
              {
                label: "Routes calls to",
                value: (
                  <Link href={`/admin/departments/${w.department_id}`} className="text-indigo-600 hover:text-indigo-500">
                    {w.department_name}
                  </Link>
                ),
              },
              { label: "Button", value: (
                <span className="inline-flex items-center gap-2">
                  <span className="inline-block h-4 w-4 rounded" style={{ backgroundColor: w.accent_color }} aria-hidden />
                  {w.button_label}
                </span>
              ) },
              { label: "Greeting", value: w.greeting || "—" },
              { label: "Created", value: formatDateTime(w.created_at) },
              { label: "Last updated", value: formatDateTime(w.updated_at) },
            ]}
          />
        </Card>
        <Card title="Allowed websites">
          <ul className="space-y-1 font-mono text-sm text-slate-800">
            {w.allowed_origins.map((o) => (
              <li key={o}>{o}</li>
            ))}
          </ul>
        </Card>
        <Card title="Caller form">
          {w.caller_form.enabled ? (
            <DescriptionList
              items={[
                { label: "Name", value: modeLabel[w.caller_form.name] },
                { label: "Phone", value: modeLabel[w.caller_form.phone] },
                { label: "Email", value: modeLabel[w.caller_form.email] },
              ]}
            />
          ) : (
            <p className="text-sm text-slate-600">Off — callers only accept the recording consent before calling.</p>
          )}
        </Card>
      </div>
    </>
  );
}
