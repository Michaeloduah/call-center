import type { Metadata } from "next";

import { PlatformSettingsForm } from "@/app/platform/settings/PlatformSettingsForm";
import { Card } from "@/components/ui/Card";
import { Notice } from "@/components/ui/Notice";
import { PageHeader } from "@/components/ui/PageHeader";
import { settingsApi } from "@/lib/api/settings";
import { formatDateTime } from "@/lib/format";
import { requireRole } from "@/lib/server/session";

export const metadata: Metadata = { title: "Platform settings · CallCenter" };

export default async function PlatformSettingsPage({ searchParams }: PageProps<"/platform/settings">) {
  await requireRole("platform_admin");
  const [settings, sp] = await Promise.all([settingsApi.getPlatform(), searchParams]);
  return (
    <>
      <PageHeader title="Platform settings" description={`Last updated ${formatDateTime(settings.updated_at)}`} />
      <Notice value={sp.notice} />
      <div className="max-w-2xl">
        <Card>
          <PlatformSettingsForm settings={settings} />
        </Card>
      </div>
    </>
  );
}
