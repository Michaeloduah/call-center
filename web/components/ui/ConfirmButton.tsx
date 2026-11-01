"use client";

import { SubmitButton } from "@/components/ui/SubmitButton";

/**
 * A one-button form for a destructive or state-changing server action. Asks for confirmation
 * in the browser before submitting.
 */
export function ConfirmButton({
  action,
  confirmMessage,
  children,
  pendingLabel = "Working…",
  variant = "secondary",
}: {
  action: () => Promise<void>;
  confirmMessage: string;
  children: React.ReactNode;
  pendingLabel?: string;
  variant?: "primary" | "secondary" | "danger" | "ghost";
}) {
  return (
    <form
      action={action}
      onSubmit={(e) => {
        if (!window.confirm(confirmMessage)) e.preventDefault();
      }}
    >
      <SubmitButton variant={variant} pendingLabel={pendingLabel}>
        {children}
      </SubmitButton>
    </form>
  );
}
