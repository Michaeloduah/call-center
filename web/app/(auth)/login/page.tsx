import type { Metadata } from "next";
import Link from "next/link";

import { LoginForm } from "@/app/(auth)/login/LoginForm";
import { Alert } from "@/components/ui/Alert";
import { Notice } from "@/components/ui/Notice";
import { param } from "@/lib/format";

export const metadata: Metadata = { title: "Sign in · CallCenter" };

export default async function LoginPage({ searchParams }: PageProps<"/login">) {
  const sp = await searchParams;
  return (
    <>
      <h1 className="mb-6 text-xl font-semibold text-slate-900">Sign in</h1>
      <Notice value={sp.notice} />
      {param(sp.expired) ? (
        <div className="mb-6">
          <Alert tone="info">Please sign in to continue.</Alert>
        </div>
      ) : null}
      <LoginForm next={param(sp.next)} />
      <p className="mt-6 text-center text-sm text-slate-600">
        New to CallCenter?{" "}
        <Link href="/signup" className="font-medium text-indigo-600 hover:text-indigo-500">
          Create an account
        </Link>
      </p>
    </>
  );
}
