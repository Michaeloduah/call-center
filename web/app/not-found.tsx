import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 text-center">
      <p className="text-sm font-semibold text-indigo-600">404</p>
      <h1 className="mt-2 text-2xl font-semibold text-slate-900">Page not found</h1>
      <p className="mt-2 text-sm text-slate-600">The page you are looking for does not exist or you do not have access to it.</p>
      <Link href="/" className="mt-6 text-sm font-medium text-indigo-600 hover:text-indigo-500">
        Go home
      </Link>
    </div>
  );
}
