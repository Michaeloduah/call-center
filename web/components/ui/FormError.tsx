import { Alert } from "@/components/ui/Alert";
import type { FormState } from "@/lib/forms";

/** Shows the form-level error message from a failed server action. */
export function FormError({ state }: { state: FormState }) {
  return state.status === "error" && state.message ? <Alert tone="error">{state.message}</Alert> : null;
}
