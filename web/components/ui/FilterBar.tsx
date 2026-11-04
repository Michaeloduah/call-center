import { buttonClass } from "@/components/ui/Button";
import { inputClass } from "@/components/ui/Field";

/** GET form for list filtering. Submitting resets to page 1 because "page" is not included. */
export function FilterBar({
  action,
  search,
  selects = [],
}: {
  action: string;
  search?: { value: string; placeholder: string };
  selects?: { name: string; label: string; value: string; options: { value: string; label: string }[] }[];
}) {
  return (
    <form action={action} method="get" className="mb-4 flex flex-wrap items-end gap-3" role="search">
      {search ? (
        <div className="min-w-56 flex-1">
          <label htmlFor="q" className="sr-only">
            Search
          </label>
          <input id="q" name="q" type="search" defaultValue={search.value} placeholder={search.placeholder} className={inputClass} />
        </div>
      ) : null}
      {selects.map((s) => (
        <div key={s.name}>
          <label htmlFor={s.name} className="sr-only">
            {s.label}
          </label>
          <select id={s.name} name={s.name} defaultValue={s.value} className={`${inputClass} bg-white`}>
            {s.options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
      ))}
      <button type="submit" className={buttonClass("secondary")}>
        Filter
      </button>
    </form>
  );
}
