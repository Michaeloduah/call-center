"use client";

import { useEffect, useRef, useState } from "react";

import type { ChatMessage } from "@/lib/types";

const MAX_LENGTH = 1000;

/** The text conversation of a call. `me` is the side this screen belongs to. */
export function ChatPanel({
  messages,
  me,
  onSend,
  sending,
  error,
  accent,
}: {
  messages: ChatMessage[];
  me: "caller" | "agent";
  onSend: (body: string) => Promise<boolean>;
  sending: boolean;
  error: string | null;
  accent?: string;
}) {
  const [draft, setDraft] = useState("");
  const list = useRef<HTMLUListElement>(null);

  useEffect(() => {
    list.current?.scrollTo({ top: list.current.scrollHeight });
  }, [messages.length]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const body = draft.trim();
    if (!body || sending) return;
    if (await onSend(body)) setDraft("");
  };

  return (
    <section aria-label="Chat" className="flex flex-col rounded-lg ring-1 ring-slate-200">
      <ul ref={list} className="max-h-56 min-h-24 space-y-2 overflow-y-auto p-3" aria-live="polite">
        {messages.length === 0 ? <li className="text-center text-xs text-slate-400">No messages yet.</li> : null}
        {messages.map((m) => {
          const mine = m.sender === me;
          return (
            <li key={m.id} className={`flex flex-col ${mine ? "items-end" : "items-start"}`}>
              <span className="text-xs text-slate-500">{mine ? "You" : m.name}</span>
              {/* Messages are plain text: React escapes them, nothing is ever rendered as HTML. */}
              <span
                className={`max-w-[85%] whitespace-pre-wrap break-words rounded-lg px-3 py-1.5 text-left text-sm ${
                  mine ? "text-white" : "bg-slate-100 text-slate-900"
                }`}
                style={mine ? { backgroundColor: accent ?? "#4f46e5" } : undefined}
              >
                {m.body}
              </span>
            </li>
          );
        })}
      </ul>
      {error ? <p className="px-3 pb-1 text-xs text-red-600">{error}</p> : null}
      <form onSubmit={submit} className="flex gap-2 border-t border-slate-200 p-2">
        <label htmlFor="chat-message" className="sr-only">
          Message
        </label>
        <input
          id="chat-message"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          maxLength={MAX_LENGTH}
          autoComplete="off"
          placeholder="Type a message"
          className="min-w-0 flex-1 rounded-md border-0 px-3 py-1.5 text-sm ring-1 ring-inset ring-slate-300 focus:ring-2 focus:ring-inset focus:ring-indigo-600"
        />
        <button
          type="submit"
          disabled={sending || draft.trim() === ""}
          className="rounded-md px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
          style={{ backgroundColor: accent ?? "#4f46e5" }}
        >
          Send
        </button>
      </form>
    </section>
  );
}
