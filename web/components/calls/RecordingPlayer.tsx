"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import type { ActionResult } from "@/lib/forms";
import type { AccessGrant } from "@/lib/types";

/**
 * Plays or downloads one recording. The link is requested only when the button is pressed (each
 * request is audit-logged and the link expires after a few minutes), and is never put in the page.
 */
export function RecordingPlayer({ access }: { access: (download: boolean) => Promise<ActionResult<AccessGrant>> }) {
  const [url, setUrl] = useState<string | null>(null);
  const [busy, setBusy] = useState<"play" | "download" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const play = async () => {
    setBusy("play");
    setError(null);
    const res = await access(false);
    setBusy(null);
    if (res.ok) setUrl(res.data.url);
    else setError(res.message);
  };

  const download = async () => {
    setBusy("download");
    setError(null);
    const res = await access(true);
    setBusy(null);
    if (res.ok) window.location.assign(res.data.url); // the link answers with a file download
    else setError(res.message);
  };

  return (
    <div className="space-y-3">
      {error ? <Alert tone="error">{error}</Alert> : null}
      {url ? (
        <audio
          controls
          autoPlay
          src={url}
          className="w-full"
          onError={() => {
            setUrl(null);
            setError("The playback link expired or the file could not be loaded. Press Play to get a new one.");
          }}
        />
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button type="button" onClick={() => void play()} disabled={busy !== null}>
          {busy === "play" ? "Preparing…" : url ? "Play again" : "Play recording"}
        </Button>
        <Button type="button" variant="secondary" onClick={() => void download()} disabled={busy !== null}>
          {busy === "download" ? "Preparing…" : "Download"}
        </Button>
      </div>
    </div>
  );
}
