"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";

const RESET_AFTER_MS = 2000;

export function CopyButton({ text }: { text: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  return (
    <Button
      type="button"
      variant="secondary"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setState("copied");
        } catch {
          setState("failed");
        }
        setTimeout(() => setState("idle"), RESET_AFTER_MS);
      }}
    >
      {state === "copied" ? "Copied" : state === "failed" ? "Copy failed — select and copy manually" : "Copy code"}
    </Button>
  );
}
