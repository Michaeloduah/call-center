const tones = {
  success: "bg-green-50 text-green-800 ring-green-200",
  error: "bg-red-50 text-red-800 ring-red-200",
  info: "bg-sky-50 text-sky-800 ring-sky-200",
  warning: "bg-amber-50 text-amber-900 ring-amber-200",
};

export function Alert({ tone, children }: { tone: keyof typeof tones; children: React.ReactNode }) {
  return (
    <div role={tone === "error" ? "alert" : "status"} className={`rounded-md px-4 py-3 text-sm ring-1 ring-inset ${tones[tone]}`}>
      {children}
    </div>
  );
}
