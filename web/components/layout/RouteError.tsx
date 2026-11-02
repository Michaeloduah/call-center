"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";

/** Shared body for error.tsx boundaries. Details are logged server-side; users see a generic message. */
export function RouteError({ error, retry }: { error: Error & { digest?: string }; retry: () => void }) {
  return (
    <div className="space-y-4">
      <Alert tone="error">
        We couldn&apos;t load this page. Please try again.
        {error.digest ? <span className="mt-1 block text-xs opacity-75">Reference: {error.digest}</span> : null}
      </Alert>
      <Button variant="secondary" onClick={() => retry()}>
        Try again
      </Button>
    </div>
  );
}
