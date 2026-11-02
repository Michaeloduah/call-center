"use client";

import { Room, RoomEvent, Track, type RemoteTrack } from "livekit-client";
import { useCallback, useEffect, useRef, useState } from "react";

import { callStatusAction, hangUpAction } from "@/app/widget/actions";
import type { CallCreateResult, CallStatus } from "@/lib/types";

const STATUS_POLL_MS = 3000;
const ENDED: CallStatus[] = ["completed", "abandoned", "missed", "failed"];

export type Phase = "connecting" | "waiting" | "in_call" | "ended" | "error";

export interface CallSession {
  phase: Phase;
  queuePosition: number | null;
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
  const roomRef = useRef<Room | null>(null);
  const endedRef = useRef(false);
  const [phase, setPhase] = useState<Phase>("connecting");
  const [queuePosition, setQueuePosition] = useState<number | null>(null);
  const [endReason, setEndReason] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const finish = useCallback((reason: string | null) => {
    if (endedRef.current) return;
    endedRef.current = true;
    setEndReason(reason);
    setPhase("ended");
    void roomRef.current?.disconnect();
  }, []);

  // Media connection.
  useEffect(() => {
    const room = new Room();
    roomRef.current = room;
    room.on(RoomEvent.TrackSubscribed, (track: RemoteTrack) => {
      if (track.kind === Track.Kind.Audio) audioContainer.current?.appendChild(track.attach());
    });
    room.on(RoomEvent.TrackUnsubscribed, (track: RemoteTrack) => {
      track.detach().forEach((el) => el.remove());
    });
    room.on(RoomEvent.Disconnected, () => {
      // Unexpected media loss: the server ends the call after its reconnect grace period.
      if (!endedRef.current) finish("connection_lost");
    });

    let cancelled = false;
    (async () => {
      try {
        await room.connect(call.media_url, call.media_token);
        await room.localParticipant.setMicrophoneEnabled(true);
        if (!cancelled) setPhase((p) => (p === "connecting" ? "waiting" : p));
      } catch {
        if (cancelled) return;
        endedRef.current = true;
        setError("We couldn't connect your call. Please check your connection and try again.");
        setPhase("error");
        // Release the queued call so it doesn't wait for an agent nobody can hear.
        void hangUpAction(call.call_id, call.caller_token);
        void room.disconnect();
      }
    })();

    return () => {
      cancelled = true;
      void room.disconnect();
    };
  }, [call, audioContainer, finish]);

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

  const toggleMute = useCallback(async () => {
    const room = roomRef.current;
    if (!room) return;
    await room.localParticipant.setMicrophoneEnabled(muted);
    setMuted(!muted);
  }, [muted]);

  return { phase, queuePosition, endReason, muted, error, hangUp, toggleMute };
}
