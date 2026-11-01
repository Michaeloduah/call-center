import { redirect } from "next/navigation";

import { getCurrentUser, homePathFor } from "@/lib/server/session";

export default async function RootPage() {
  const user = await getCurrentUser();
  redirect(user ? homePathFor(user.role) : "/login");
}
