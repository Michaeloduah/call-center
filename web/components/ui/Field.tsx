import type { ComponentProps } from "react";

/** Input styling without a width, for inline controls that set their own width. */
const inputBaseClass =
  "rounded-md border-0 px-3 py-2 text-sm text-slate-900 shadow-sm ring-1 ring-inset ring-slate-300 " +
  "placeholder:text-slate-400 focus:ring-2 focus:ring-inset focus:ring-indigo-600 aria-[invalid=true]:ring-red-500";

const inputClass = `block w-full ${inputBaseClass}`;

interface FieldShellProps {
  id: string;
  label: string;
  error?: string;
  hint?: string;
  children: React.ReactNode;
}

function FieldShell({ id, label, error, hint, children }: FieldShellProps) {
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-sm font-medium text-slate-800">
        {label}
      </label>
      {children}
      {error ? (
        <p id={`${id}-error`} className="text-sm text-red-600">
          {error}
        </p>
      ) : hint ? (
        <p id={`${id}-hint`} className="text-sm text-slate-500">
          {hint}
        </p>
      ) : null}
    </div>
  );
}

type TextFieldProps = ComponentProps<"input"> & { name: string; label: string; error?: string; hint?: string };

export function TextField({ name, label, error, hint, id = name, ...props }: TextFieldProps) {
  return (
    <FieldShell id={id} label={label} error={error} hint={hint}>
      <input
        id={id}
        name={name}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
        className={inputClass}
        {...props}
      />
    </FieldShell>
  );
}

type TextAreaFieldProps = ComponentProps<"textarea"> & { name: string; label: string; error?: string; hint?: string };

export function TextAreaField({ name, label, error, hint, id = name, ...props }: TextAreaFieldProps) {
  return (
    <FieldShell id={id} label={label} error={error} hint={hint}>
      <textarea
        id={id}
        name={name}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
        className={inputClass}
        {...props}
      />
    </FieldShell>
  );
}

type SelectFieldProps = ComponentProps<"select"> & {
  name: string;
  label: string;
  error?: string;
  hint?: string;
  options: { value: string; label: string }[];
};

export function SelectField({ name, label, error, hint, options, id = name, ...props }: SelectFieldProps) {
  return (
    <FieldShell id={id} label={label} error={error} hint={hint}>
      <select
        id={id}
        name={name}
        aria-invalid={error ? true : undefined}
        className={`${inputClass} bg-white`}
        {...props}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </FieldShell>
  );
}

export { inputBaseClass, inputClass };
