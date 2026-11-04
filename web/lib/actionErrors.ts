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
  plan_in_use: "Tenants are subscribed to this plan. Deactivate it instead of deleting it.",
  price_in_use: "Tenants are subscribed at this price. Deactivate it instead of deleting it.",
  free_plan: "The free plan cannot be deleted or deactivated, and has no prices.",
  already_subscribed: "You are already subscribed at this price.",
  nothing_to_cancel: "You are on the free plan, so there is nothing to cancel.",
  price_not_synced: "This price is not available for checkout yet.",
  provider_unavailable: "The payment provider is not available right now. Please try again later.",
  plan_limit: "Your plan does not allow that. Upgrade your plan to continue.",
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
