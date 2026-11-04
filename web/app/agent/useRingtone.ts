"use client";

import { useCallback, useEffect, useRef } from "react";

const BEEP_HZ = 880;
const BEEP_SECONDS = 0.35;
const BEEP_GAIN = 0.12;
const REPEAT_MS = 2000;

/**
 * Rings while an offer is on screen. Browsers only allow sound after a user gesture, so the
 * audio context is created by unlock(), which the console calls from the "Go available" click.
 */
export function useRingtone(ringing: boolean): { unlock: () => void } {
  const context = useRef<AudioContext | null>(null);

  const unlock = useCallback(() => {
    context.current ??= new AudioContext();
    void context.current.resume();
  }, []);

  useEffect(() => {
    if (!ringing) return;
    const originalTitle = document.title;
    document.title = "Incoming call · CallCenter";

    const beep = () => {
      const ctx = context.current;
      if (!ctx || ctx.state !== "running") return;
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.frequency.value = BEEP_HZ;
      gain.gain.value = BEEP_GAIN;
      osc.connect(gain).connect(ctx.destination);
      osc.start();
      osc.stop(ctx.currentTime + BEEP_SECONDS);
    };
    beep();
    const timer = setInterval(beep, REPEAT_MS);
    return () => {
      clearInterval(timer);
      document.title = originalTitle;
    };
  }, [ringing]);

  return { unlock };
}
