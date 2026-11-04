"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { callStatusAction, hangUpAction } from "@/app/widget/actions";
import { type MediaCredentials, useLiveKitRoom } from "@/lib/media/useLiveKitRoom";
import type { CallCreateResult, CallStatus } from "@/lib/types";

const STATUS_POLL_MS = 3000;
const ENDED: CallStatus[] = ["completed", "abandoned", "missed", "failed"];

export type Phase = "connecting" | "waiting" | "in_call" | "ended" | "error";

export interface CallSession {
  phase: Phase;
  queuePosition: number | null;
  /** An agent has been offered the call and has not answered yet. */
  agentRinging: boolean;
  endReason: string | null;
  muted: boolean;
  error: string | null;
  hangUp: () => Promise<void>;
  toggleMute: () => Promise<void>;
}

/**
 * Joins the call's media room, keeps the caller's status in sync with the API, and plays the
 * agent's audio when they join. The API is the source of truth for the call's state; the media
 * connection only carries audio.
 */
export function useCallSession(call: CallCreateResult, audioContainer: React.RefObject<HTMLDivElement | null>): CallSession {
  const endedRef = useRef(false);
  const [credentials, setCredentials] = useState<MediaCredentials | null>({ url: call.media_url, token: call.media_token });
  const [phase, setPhase] = useState<Phase>("connecting");
  const [queuePosition, setQueuePosition] = useState<number | null>(null);
  const [agentRinging, setAgentRinging] = useState(false);
  const [endReason, setEndReason] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const finish = useCallback((reason: string | null) => {
    if (endedRef.current) return;
    endedRef.current = true;
    setEndReason(reason);
    setPhase("ended");
    setCredentials(null); // leaves the media room
  }, []);

  const { muted, toggleMute } = useLiveKitRoom({
    credentials,
    audioContainer,
    onConnected: () => setPhase((p) => (p === "connecting" ? "waiting" : p)),
    onConnectFailed: () => {
      endedRef.current = true;
      setError("We couldn't connect your call. Please check your connection and try again.");
      setPhase("error");
      setCredentials(null);
      // Release the queued call so it doesn't wait for an agent nobody can hear.
      void hangUpAction(call.call_id, call.caller_token);
    },
    // Unexpected media loss: the server ends the call after its reconnect grace period.
    onLost: () => finish("connection_lost"),
  });

  // Status polling.
  useEffect(() => {
    const poll = async () => {
      if (endedRef.current) return;
      const res = await callStatusAction(call.call_id, call.caller_token);
      if (endedRef.current) return;
      if (!res.ok) {
        if (res.code === "invalid_caller_token") finish("expired");
        return; // transient failures: try again on the next tick
      }
      const { status, queue_position, end_reason } = res.call;
      if (ENDED.includes(status)) {
        finish(end_reason);
      } else if (status === "active") {
        setPhase("in_call");
      } else {
        setQueuePosition(queue_position);
        setAgentRinging(status === "ringing");
        setPhase((p) => (p === "connecting" ? p : "waiting"));
      }
    };
    void poll();
    const timer = setInterval(() => void poll(), STATUS_POLL_MS);
    return () => clearInterval(timer);
  }, [call, finish]);

  const hangUp = useCallback(async () => {
    await hangUpAction(call.call_id, call.caller_token);
    finish("caller_hung_up");
  }, [call, finish]);

  return { phase, queuePosition, agentRinging, endReason, muted, error, hangUp, toggleMute };
}
