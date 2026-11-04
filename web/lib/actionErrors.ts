/**
 * Messages for errors returned by one-click actions (?error=code). Only whitelisted codes are
 * rendered so the query string cannot inject text.
 */
const actionErrors: Record<string, string> = {
  tenant_not_suspended: "Suspend the tenant before deleting it.",
  last_admin: "Your organisation must keep at least one active admin.",
  forbidden: "You cannot perform this action on your own account.",
  not_invited: "Only users who have not accepted their invitation can be re-invited.",
  not_found: "That item no longer exists.",
  recording_unavailable: "This recording is not available.",
  callback_taken: "Someone else already claimed this callback.",
  callback_state: "This callback cannot be changed right now.",
  department_in_use:
    "This department has widgets or call history. Deactivate it instead, or move its widgets to another department first.",
};

export function actionErrorText(code: string | string[] | undefined): string | null {
  const key = Array.isArray(code) ? code[0] : code;
  if (!key) return null;
  return actionErrors[key] ?? "The action could not be completed. Please try again.";
}
