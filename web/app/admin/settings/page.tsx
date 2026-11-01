import type { Metadata } from "next";

import { TenantSettingsForm } from "@/app/admin/settings/TenantSettingsForm";
import { Card } from "@/components/ui/Card";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { settingsApi } from "@/lib/api/settings";
import { formatDateTime } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Settings · CallCenter" };

export default async function TenantSettingsPage({ searchParams }: PageProps<"/admin/settings">) {
  await requireRole("tenant_admin");
  const [settings, sp] = await Promise.all([settingsApi.getTenant(), searchParams]);
  return (
    <>
      <PageHeader title="Settings" description={`Last updated ${formatDateTime(settings.updated_at)}`} />
      <Notice value={sp.notice} />
      <div className="max-w-2xl">
        <Card>
          <TenantSettingsForm settings={settings} />
        </Card>
      </div>
    </>
  );
}
