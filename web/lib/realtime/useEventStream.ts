"use client";

import { useEffect, useRef, useState } from "react";

const RETRY_AFTER_FAILURE_MS = 5000;

/**
 * Subscribes to a Server-Sent Events endpoint and calls onChange whenever the server says
 * something changed, and again after every (re)connect so nothing missed in between is lost.
 * The events carry no data: onChange is expected to re-read the state. Returns whether the
 * stream is currently connected.
 */
export function useEventStream(url: string, onChange: () => void): boolean {
  const [connected, setConnected] = useState(false);
  const callback = useRef(onChange);

  useEffect(() => {
    callback.current = onChange;
  });

  useEffect(() => {
    let source: EventSource | null = null;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;

    const open = () => {
      source = new EventSource(url);
      source.addEventListener("ready", () => {
        setConnected(true);
        callback.current();
      });
      source.addEventListener("changed", () => callback.current());
      source.onerror = () => {
        setConnected(false);
        // The browser retries dropped streams itself; a refused one (CLOSED) it gives up on.
        if (source?.readyState === EventSource.CLOSED && !stopped) {
          source = null;
          retry = setTimeout(open, RETRY_AFTER_FAILURE_MS);
        }
      };
    };
    open();

    return () => {
      stopped = true;
      clearTimeout(retry);
      source?.close();
    };
  }, [url]);

  return connected;
}
