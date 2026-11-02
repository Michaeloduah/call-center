import type { Metadata } from "next";
import Link from "next/link";

import { SignupForm } from "@/app/(auth)/signup/SignupForm";

export const metadata: Metadata = { title: "Create account · CallCenter" };

export default function SignupPage() {
  return (
    <>
      <h1 className="text-xl font-semibold text-slate-900">Create your account</h1>
      <p className="mb-6 mt-1 text-sm text-slate-600">Start on the free plan. No card required.</p>
      <SignupForm />
      <p className="mt-6 text-center text-sm text-slate-600">
        Already have an account?{" "}
        <Link href="/login" className="font-medium text-indigo-600 hover:text-indigo-500">
          Sign in
        </Link>
      </p>
    </>
  );
}
