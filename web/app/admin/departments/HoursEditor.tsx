"use client";

import { useActionState, useState } from "react";

import { Button, LinkButton } from "@/components/ui/Button";
import { inputBaseClass } from "@/components/ui/Field";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { weekdayLabels } from "@/lib/format";
import { type FormState, initialFormState } from "@/lib/forms";
import type { HoursRange } from "@/lib/types";

const presets: { label: string; rows: () => HoursRange[] }[] = [
  { label: "Mon–Fri, 9:00–17:00", rows: () => [1, 2, 3, 4, 5].map((d) => ({ weekday: d, opens: "09:00", closes: "17:00" })) },
  { label: "Open 24/7", rows: () => [0, 1, 2, 3, 4, 5, 6].map((d) => ({ weekday: d, opens: "00:00", closes: "24:00" })) },
  { label: "Clear (always closed)", rows: () => [] },
];

/**
 * Edits a department's weekly schedule. Rows are submitted as JSON; the API validates formats and
 * overlaps and returns errors keyed by row index (business_hours.N).
 *
 * The row controls deliberately live outside the <form>: React resets a form's fields after its
 * action runs, which would desync these controlled inputs from state after a failed save.
 */
export function HoursEditor({
  initial,
  timezone,
  action: serverAction,
  cancelHref,
}: {
  initial: HoursRange[];
  timezone: string;
  action: (prev: FormState, formData: FormData) => Promise<FormState>;
  cancelHref: string;
}) {
  const [state, action] = useActionState(serverAction, initialFormState);
  const [rows, setRows] = useState<HoursRange[]>(initial);

  const update = (i: number, patch: Partial<HoursRange>) => setRows((prev) => prev.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  const remove = (i: number) => setRows((prev) => prev.filter((_, j) => j !== i));
  const add = () => setRows((prev) => [...prev, { weekday: 1, opens: "09:00", closes: "17:00" }]);

  return (
    <div className="space-y-5">
      <FormError state={state} />
      <p className="text-sm text-slate-600">
        Times are in <span className="font-medium text-slate-900">{timezone}</span>. Use 24:00 to mean end of day. Outside
        these hours, callers will be offered a callback form.
      </p>
      <div className="flex flex-wrap gap-2">
        {presets.map((p) => (
          <Button key={p.label} type="button" variant="secondary" onClick={() => setRows(p.rows())}>
            {p.label}
          </Button>
        ))}
      </div>
      {rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-900 ring-1 ring-amber-200">
          No opening hours: this department is always closed.
        </p>
      ) : (
        <ul className="space-y-3">
          {rows.map((row, i) => {
            const error = state.fieldErrors?.[`business_hours.${i}`];
            return (
              <li key={i} className="space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <select
                    aria-label={`Day for range ${i + 1}`}
                    value={row.weekday}
                    onChange={(e) => update(i, { weekday: Number(e.target.value) })}
                    aria-invalid={error ? true : undefined}
                    className={`${inputBaseClass} w-40 bg-white`}
                  >
                    {weekdayLabels.map((label, d) => (
                      <option key={label} value={d}>
                        {label}
                      </option>
                    ))}
                  </select>
                  <input
                    aria-label={`Opens for range ${i + 1}`}
                    value={row.opens}
                    onChange={(e) => update(i, { opens: e.target.value })}
                    placeholder="09:00"
                    inputMode="numeric"
                    aria-invalid={error ? true : undefined}
                    className={`${inputBaseClass} w-24`}
                  />
                  <span className="text-sm text-slate-500">to</span>
                  <input
                    aria-label={`Closes for range ${i + 1}`}
                    value={row.closes}
                    onChange={(e) => update(i, { closes: e.target.value })}
                    placeholder="17:00"
                    inputMode="numeric"
                    aria-invalid={error ? true : undefined}
                    className={`${inputBaseClass} w-24`}
                  />
                  <Button type="button" variant="ghost" onClick={() => remove(i)} aria-label={`Remove range ${i + 1}`}>
                    Remove
                  </Button>
                </div>
                {error ? <p className="text-sm text-red-600">{error}</p> : null}
              </li>
            );
          })}
        </ul>
      )}
      <Button type="button" variant="secondary" onClick={add}>
        Add time range
      </Button>
      <form action={action} className="flex gap-3 border-t border-slate-200 pt-5">
        <input type="hidden" name="hours" value={JSON.stringify(rows)} />
        <SubmitButton>Save schedule</SubmitButton>
        <LinkButton href={cancelHref} variant="secondary">
          Cancel
        </LinkButton>
      </form>
    </div>
  );
}
