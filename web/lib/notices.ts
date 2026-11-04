/**
 * Success messages shown after a redirect (?notice=key). Only whitelisted keys are rendered,
 * so the query string can never inject arbitrary text into the page.
 */
export const notices = {
  "tenant-created": "Tenant created and the admin was invited.",
  "tenant-created-no-email": "Tenant created, but the invitation email could not be sent. Resend it from the tenant's user list.",
  "tenant-updated": "Tenant updated.",
  "tenant-suspended": "Tenant suspended. Its users have been signed out.",
  "tenant-activated": "Tenant activated.",
  "tenant-deleted": "Tenant deleted.",
  "settings-saved": "Settings saved.",
  "user-invited": "User invited. They will receive an email to set their password.",
  "user-invited-no-email": "User created, but the invitation email could not be sent. Use “Resend invitation”.",
  "user-updated": "User updated.",
  "user-disabled": "User disabled and signed out.",
  "user-enabled": "User enabled.",
  "user-deleted": "User deleted.",
  "invite-resent": "Invitation sent again.",
  "invite-resent-no-email": "A new invitation was created, but the email could not be sent.",
  "department-created": "Department created.",
  "department-updated": "Department updated.",
  "department-deleted": "Department deleted.",
  "members-saved": "Agents updated.",
  "supervisors-saved": "Supervisors updated.",
  "hours-saved": "Business hours saved.",
  "widget-created": "Widget created. Add the embed code below to your website.",
  "widget-updated": "Widget updated.",
  "widget-deleted": "Widget deleted. Its call history is kept.",
  "recording-deleted": "Recording deleted. The platform administrator can restore it.",
  "recording-restored": "Recording restored. Its tenant can see it again.",
  "callback-claimed": "Callback claimed. It is yours to call back.",
  "callback-released": "Callback released back to the queue.",
  "callback-completed": "Callback completed.",
  "callback-cancelled": "Callback cancelled.",
  "callback-assigned": "Callback assigned.",
  "plan-created": "Plan created. Add a price so tenants can subscribe.",
  "plan-updated": "Plan updated.",
  "plan-deleted": "Plan deleted.",
  "price-added": "Price added.",
  "price-updated": "Price updated.",
  "price-deleted": "Price deleted.",
  "price-synced": "Price is ready for checkout.",
  "policies-saved": "Cap settings saved.",
  "subscription-cancelled": "Your subscription will not renew. You keep your plan until the end of the current period.",
  "checkout-return": "Thanks! We are confirming your payment. Your plan updates as soon as the payment provider confirms it.",
  "password-reset": "Your password has been changed. Please sign in.",
  "invite-accepted": "Your account is ready. Please sign in.",
  "email-verified": "Your email is verified. Please sign in.",
  "signed-out": "You have been signed out.",
} as const;

export type NoticeKey = keyof typeof notices;

export function noticeText(key: string | undefined): string | null {
  return key && key in notices ? notices[key as NoticeKey] : null;
}
