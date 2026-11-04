import { relayEventStream } from "@/lib/server/stream";

export const dynamic = "force-dynamic";

export function GET(request: Request) {
  return relayEventStream(request, "/v1/live/events");
}
