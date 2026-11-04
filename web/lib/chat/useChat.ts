"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import type { ActionResult } from "@/lib/forms";
import type { ChatMessage } from "@/lib/types";

/** The chat topic the server uses for message packets on the media connection. */
const CHAT_TOPIC = "chat";

export interface ChatSource {
  load: () => Promise<ActionResult<ChatMessage[]>>;
  send: (body: string) => Promise<ActionResult<ChatMessage>>;
}

function isChatMessage(value: unknown): value is ChatMessage {
  if (typeof value !== "object" || value === null) return false;
  const m = value as Record<string, unknown>;
  return (
    typeof m.id === "string" &&
    (m.sender === "caller" || m.sender === "agent") &&
    typeof m.name === "string" &&
    typeof m.body === "string" &&
    typeof m.sent_at === "string"
  );
}

/** Adds messages, dropping ones already shown (a message can arrive live and from the history). */
function merge(current: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  const known = new Set(current.map((m) => m.id));
  const added = incoming.filter((m) => !known.has(m.id));
  if (added.length === 0) return current;
  return [...current, ...added].sort((a, b) => a.sent_at.localeCompare(b.sent_at) || a.id.localeCompare(b.id));
}

/**
 * Chat for one call. Messages arrive over the media connection (receive) and, for catching up
 * after a reload or a missed packet, from the history endpoint. The server saves every message
 * before delivering it, so the history is always complete.
 */
export function useChat(callId: string | null, active: boolean, source: ChatSource) {
  const [state, setState] = useState<{ callId: string | null; messages: ChatMessage[] }>({ callId, messages: [] });
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sourceRef = useRef(source);

  useEffect(() => {
    sourceRef.current = source;
  });

  const add = useCallback(
    (incoming: ChatMessage[]) =>
      setState((s) => (s.callId === callId ? { callId, messages: merge(s.messages, incoming) } : { callId, messages: merge([], incoming) })),
    [callId],
  );

  useEffect(() => {
    if (!callId || !active) return;
    let cancelled = false;
    void sourceRef.current.load().then((res) => {
      if (!cancelled && res.ok) add(res.data);
    });
    return () => {
      cancelled = true;
    };
  }, [callId, active, add]);

  const receive = useCallback(
    (topic: string | undefined, payload: Uint8Array) => {
      if (topic !== CHAT_TOPIC) return;
      try {
        const message: unknown = JSON.parse(new TextDecoder().decode(payload));
        if (isChatMessage(message)) add([message]);
      } catch {
        // Not a chat packet we understand: ignore it.
      }
    },
    [add],
  );

  /** Sends a message. Resolves true when it was accepted. */
  const send = useCallback(
    async (body: string): Promise<boolean> => {
      setSending(true);
      setError(null);
      const res = await sourceRef.current.send(body);
      setSending(false);
      if (!res.ok) {
        setError(res.message);
        return false;
      }
      add([res.data]);
      return true;
    },
    [add],
  );

  const messages = state.callId === callId ? state.messages : [];
  return { messages, sending, error, send, receive };
}
