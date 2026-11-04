"use server";

import { liveApi } from "@/lib/api/agent";
import type { ActionResult } from "@/lib/forms";
import { attempt } from "@/lib/server/actions";
import type { Board } from "@/lib/types";

export async function boardAction(): Promise<ActionResult<Board>> {
  return attempt(() => liveApi.board());
}
