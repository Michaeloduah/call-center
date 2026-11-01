import Link from "next/link";

/** Previous/next pagination that preserves the current filters in the query string. */
export function Pagination({
  basePath,
  page,
  perPage,
  total,
  query,
}: {
  basePath: string;
  page: number;
  perPage: number;
  total: number;
  query: Record<string, string>;
}) {
  const pages = Math.max(1, Math.ceil(total / perPage));
  if (pages <= 1) return null;
  const href = (p: number) => {
    const params = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== ""));
    params.set("page", String(p));
    return `${basePath}?${params.toString()}`;
  };
  const linkClass = "rounded-md px-3 py-1.5 text-sm ring-1 ring-inset ring-slate-300 bg-white hover:bg-slate-50";
  return (
    <nav aria-label="Pagination" className="mt-4 flex items-center justify-between text-sm text-slate-600">
      <span>
        Page {page} of {pages} · {total} total
      </span>
      <div className="flex gap-2">
        {page > 1 ? (
          <Link href={href(page - 1)} className={linkClass}>
            Previous
          </Link>
        ) : null}
        {page < pages ? (
          <Link href={href(page + 1)} className={linkClass}>
            Next
          </Link>
        ) : null}
      </div>
    </nav>
  );
}
