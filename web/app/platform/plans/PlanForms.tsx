"use client";

import { useActionState } from "react";

import { createPlanAction, createPriceAction, updatePlanAction } from "@/app/platform/plans/actions";
import { LinkButton } from "@/components/ui/Button";
import { SelectField, TextAreaField, TextField } from "@/components/ui/Field";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { initialFormState } from "@/lib/forms";
import type { Plan } from "@/lib/types";

const GB = 1024 ** 3;

function Checkbox({ name, label, defaultChecked }: { name: string; label: string; defaultChecked: boolean }) {
  return (
    <label className="flex items-center gap-2 text-sm text-slate-800">
      <input type="checkbox" name={name} defaultChecked={defaultChecked} className="h-4 w-4 rounded border-slate-300 text-indigo-600" />
      {label}
    </label>
  );
}

/** Create or edit a plan. Empty limits mean unlimited. */
export function PlanForm({ plan }: { plan?: Plan }) {
  const [state, action] = useActionState(plan ? updatePlanAction.bind(null, plan.id) : createPlanAction, initialFormState);
  const v = state.values;
  const e = state.fieldErrors ?? {};
  const num = (name: string, current: number | null | undefined) => v?.[name] ?? (current === null || current === undefined ? "" : String(current));
  const checked = (name: string, current: boolean) => (v ? v[name] === "on" : current);
  const hint = "Leave empty for unlimited.";

  return (
    <form action={action} className="space-y-6" noValidate>
      <FormError state={state} />
      <fieldset className="space-y-5">
        <TextField name="code" label="Code" required disabled={plan?.is_free} defaultValue={v?.code ?? plan?.code} error={e.code} hint="Lowercase letters, digits, hyphens. Cannot be changed on the free plan." />
        <TextField name="name" label="Name" required defaultValue={v?.name ?? plan?.name} error={e.name} />
        <TextAreaField name="description" label="Description" rows={2} defaultValue={v?.description ?? plan?.description} error={e.description} />
        {plan?.is_free ? null : (
          <div className="space-y-2">
            <Checkbox name="is_public" label="Public (tenants can choose it)" defaultChecked={checked("is_public", plan?.is_public ?? true)} />
            <Checkbox name="is_active" label="Active" defaultChecked={checked("is_active", plan?.is_active ?? true)} />
          </div>
        )}
      </fieldset>
      <fieldset className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <legend className="mb-3 text-sm font-semibold text-slate-900">Limits</legend>
        <TextField name="max_agents" label="Agent seats" inputMode="numeric" hint={hint} defaultValue={num("max_agents", plan?.max_agents)} error={e.max_agents} />
        <TextField name="max_call_minutes" label="Call minutes per month" inputMode="numeric" hint={hint} defaultValue={num("max_call_minutes", plan?.max_call_minutes_per_month)} error={e.max_call_minutes} />
        <TextField name="max_storage_gb" label="Recording storage (GB)" inputMode="decimal" hint={hint}
          defaultValue={v?.max_storage_gb ?? (plan?.max_storage_bytes == null ? "" : String(plan.max_storage_bytes / GB))} error={e.max_storage_gb} />
        <TextField name="max_departments" label="Departments" inputMode="numeric" hint={hint} defaultValue={num("max_departments", plan?.max_departments)} error={e.max_departments} />
        <TextField name="max_widgets" label="Widgets" inputMode="numeric" hint={hint} defaultValue={num("max_widgets", plan?.max_widgets)} error={e.max_widgets} />
      </fieldset>
      <div className="flex gap-3">
        <SubmitButton pendingLabel="Saving…">{plan ? "Save plan" : "Create plan"}</SubmitButton>
        <LinkButton href={plan ? `/platform/plans/${plan.id}` : "/platform/plans"} variant="secondary">
          Cancel
        </LinkButton>
      </div>
    </form>
  );
}

/** Adds a price (currency, interval, amount and overage rates) to a paid plan. Amounts are in major units, e.g. 49.00. */
export function AddPriceForm({ planId }: { planId: string }) {
  const [state, action] = useActionState(createPriceAction.bind(null, planId), initialFormState);
  const v = state.values;
  const e = state.fieldErrors ?? {};
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-3">
        <TextField name="currency" label="Currency" required maxLength={3} placeholder="USD" defaultValue={v?.currency} error={e.currency}
          hint="NGN is charged through Paystack, every other currency through Stripe." />
        <SelectField name="interval" label="Billed" defaultValue={v?.interval ?? "monthly"} error={e.interval}
          options={[{ value: "monthly", label: "Monthly" }, { value: "yearly", label: "Yearly" }]} />
        <TextField name="amount" label="Price" required inputMode="decimal" placeholder="49.00" defaultValue={v?.amount} error={e.amount} />
        <TextField name="overage_minute" label="Extra call minute" inputMode="decimal" placeholder="0.05" defaultValue={v?.overage_minute} error={e.overage_minute_minor ?? e.overage_minute}
          hint="Charged per minute beyond the plan, if the tenant chooses to pay for extra usage." />
        <TextField name="overage_gb" label="Extra storage per GB" inputMode="decimal" placeholder="0.50" defaultValue={v?.overage_gb} error={e.overage_gb_minor ?? e.overage_gb} />
      </div>
      <SubmitButton variant="secondary" pendingLabel="Adding…">Add price</SubmitButton>
    </form>
  );
}
