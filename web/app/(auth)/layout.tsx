import Link from "next/link";

export default function AuthLayout({ children }: LayoutProps<"/">) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 py-12">
      <Link href="/" className="mb-8 text-lg font-semibold text-slate-900">
        CallCenter
      </Link>
      <div className="w-full max-w-md rounded-lg bg-white p-8 shadow-sm ring-1 ring-slate-200">{children}</div>
    </div>
  );
}
