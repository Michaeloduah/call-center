/** One usage bar: how much of a limit is used. A null limit means unlimited. */
export function UsageMeter({ label, used, limit, format }: { label: string; used: number; limit: number | null; format: (n: number) => string }) {
  const percent = limit && limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0;
  const tone = percent >= 100 ? "bg-red-500" : percent >= 80 ? "bg-amber-500" : "bg-indigo-600";
  return (
    <div>
      <div className="flex items-baseline justify-between text-sm">
        <span className="font-medium text-slate-800">{label}</span>
        <span className="text-slate-600">
          {format(used)} {limit === null ? "· unlimited" : `of ${format(limit)}`}
        </span>
      </div>
      {limit !== null ? (
        <div
          role="progressbar"
          aria-label={label}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent}
          className="mt-1.5 h-2 overflow-hidden rounded-full bg-slate-100"
        >
          <div className={`h-full ${tone}`} style={{ width: `${percent}%` }} />
        </div>
      ) : null}
    </div>
  );
}
