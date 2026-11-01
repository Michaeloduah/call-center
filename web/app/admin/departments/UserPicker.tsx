"use client";

import { useActionState, useMemo, useState } from "react";

import { LinkButton } from "@/components/ui/Button";
import { inputBaseClass } from "@/components/ui/Field";
import { FormError } from "@/components/ui/FormError";
import { SubmitButton } from "@/components/ui/SubmitButton";
import { roleLabels } from "@/lib/format";
import { type FormState, initialFormState } from "@/lib/forms";
import type { User } from "@/lib/types";

/**
 * Checkbox list for choosing which users are linked to a department. Saves the full selection.
 *
 * The checkboxes deliberately live outside the <form>: React resets a form's fields after its
 * action runs, which would desync these controlled checkboxes from state after a failed save.
 */
export function UserPicker({
  users,
  selected,
  action: serverAction,
  cancelHref,
  emptyMessage,
}: {
  users: User[];
  selected: string[];
  action: (prev: FormState, formData: FormData) => Promise<FormState>;
  cancelHref: string;
  emptyMessage: string;
}) {
  const [state, action] = useActionState(serverAction, initialFormState);
  const [checked, setChecked] = useState(() => new Set(selected));
  const [query, setQuery] = useState("");

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? users.filter((u) => u.full_name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q)) : users;
  }, [users, query]);

  const toggle = (id: string) =>
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <div className="space-y-4">
      <FormError state={state} />
      {state.fieldErrors?.user_ids ? <p className="text-sm text-red-600">{state.fieldErrors.user_ids}</p> : null}
      {users.length === 0 ? (
        <p className="text-sm text-slate-500">{emptyMessage}</p>
      ) : (
        <>
          <div className="flex items-center justify-between gap-4">
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Filter by name or email"
              aria-label="Filter users"
              className={`${inputBaseClass} w-full max-w-xs`}
            />
            <span className="text-sm text-slate-500">{checked.size} selected</span>
          </div>
          <ul className="max-h-96 divide-y divide-slate-100 overflow-y-auto rounded-md ring-1 ring-slate-200">
            {visible.map((u) => (
              <li key={u.id}>
                <label className="flex cursor-pointer items-center gap-3 px-4 py-2.5 hover:bg-slate-50">
                  <input
                    type="checkbox"
                    checked={checked.has(u.id)}
                    onChange={() => toggle(u.id)}
                    className="h-4 w-4 rounded border-slate-300 text-indigo-600"
                  />
                  <span className="flex-1 text-sm">
                    <span className="font-medium text-slate-900">{u.full_name}</span>
                    <span className="ml-2 text-slate-500">{u.email}</span>
                  </span>
                  <span className="text-xs text-slate-500">
                    {roleLabels[u.role]}
                    {u.status === "invited" ? " · invited" : ""}
                  </span>
                </label>
              </li>
            ))}
            {visible.length === 0 ? <li className="px-4 py-3 text-sm text-slate-500">No users match that filter.</li> : null}
          </ul>
        </>
      )}
      <form action={action} className="flex gap-3">
        {/* Hidden inputs carry the selection, including users filtered out of view. */}
        {[...checked].map((id) => (
          <input key={id} type="hidden" name="user_ids" value={id} />
        ))}
        <SubmitButton>Save</SubmitButton>
        <LinkButton href={cancelHref} variant="secondary">
          Cancel
        </LinkButton>
      </form>
    </div>
  );
}
