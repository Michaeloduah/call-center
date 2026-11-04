"use client";

import { useActionState } from "react";

import { savePoliciesAction } from "@/app/admin/billing/actions";
import { FormError } from "@/components/ui/FormError";
import { SelectField } from "@/components/ui/Field";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { initialFormState } from "@/lib/forms";
import type { CapPolicy } from "@/lib/types";

/** What happens when a cap is reached: block new calls, or keep going and pay for the extra. */
export function PoliciesForm({ minutes, storage, canOverage }: { minutes: CapPolicy; storage: CapPolicy; canOverage: boolean }) {
  const [state, action] = useActionState(savePoliciesAction, initialFormState);
  const options = [
    { value: "block", label: "Stop taking new calls (callers see the callback form)" },
    ...(canOverage ? [{ value: "overage", label: "Keep going and pay for the extra" }] : []),
  ];
  return (
    <form action={action} className="space-y-5" noValidate>
      <FormError state={state} />
      <SelectField
        name="minutes_cap_policy"
        label="When monthly call minutes run out"
        options={options}
        defaultValue={state.values?.minutes_cap_policy ?? minutes}
        error={state.fieldErrors?.minutes_cap_policy}
      />
      <SelectField
        name="storage_cap_policy"
        label="When recording storage is full"
        options={options}
        defaultValue={state.values?.storage_cap_policy ?? storage}
        error={state.fieldErrors?.storage_cap_policy}
        hint="Deleting recordings does not free storage: deleted recordings are kept and still count."
      />
      {!canOverage ? <p className="text-sm text-slate-500">Paying for extra usage is available on paid plans.</p> : null}
      <SubmitButton pendingLabel="Saving…">Save</SubmitButton>
    </form>
  );
}
