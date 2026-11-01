/** State returned by server actions to forms rendered with useActionState. */
export interface FormState {
  status: "idle" | "error" | "success";
  message?: string;
  /** Machine-readable API error code, for errors the UI handles specially. */
  code?: string;
  fieldErrors?: Record<string, string>;
  /** Submitted values, echoed back so inputs keep their content after a failed submit. */
  values?: Record<string, string>;
}

export const initialFormState: FormState = { status: "idle" };

/** Reads a trimmed string field from FormData. */
export function field(formData: FormData, name: string): string {
  const value = formData.get(name);
  return typeof value === "string" ? value.trim() : "";
}

/** Collects the named string fields, used to echo values back on error. Passwords are never echoed. */
export function values(formData: FormData, names: string[]): Record<string, string> {
  return Object.fromEntries(names.map((n) => [n, field(formData, n)]));
}
