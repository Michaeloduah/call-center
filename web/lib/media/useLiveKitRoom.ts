"use client";

import { Room, RoomEvent, Track, type RemoteTrack } from "livekit-client";
import { useCallback, useEffect, useRef, useState } from "react";

export interface MediaCredentials {
  url: string;
  token: string;
}

interface Options {
  /** Joins while set, leaves when it becomes null or changes. */
  credentials: MediaCredentials | null;
  /** Where the other side's audio elements are attached. */
  audioContainer: React.RefObject<HTMLDivElement | null>;
  onConnected?: () => void;
  onConnectFailed?: () => void;
  /** The media connection dropped without us asking. */
  onLost?: () => void;
}

/**
 * Joins a call room with the microphone on and plays the other side's audio. Shared by the
 * caller's widget and the agent console; call state itself always comes from the API.
 */
export function useLiveKitRoom({ credentials, audioContainer, onConnected, onConnectFailed, onLost }: Options) {
  const roomRef = useRef<Room | null>(null);
  const handlers = useRef({ onConnected, onConnectFailed, onLost });
  const [muted, setMuted] = useState(false);

  useEffect(() => {
    handlers.current = { onConnected, onConnectFailed, onLost };
  });

  const url = credentials?.url;
  const token = credentials?.token;
  useEffect(() => {
    if (!url || !token) return;
    const room = new Room();
    roomRef.current = room;
    let leaving = false;

    room.on(RoomEvent.TrackSubscribed, (track: RemoteTrack) => {
      if (track.kind === Track.Kind.Audio) audioContainer.current?.appendChild(track.attach());
    });
    room.on(RoomEvent.TrackUnsubscribed, (track: RemoteTrack) => {
      track.detach().forEach((el) => el.remove());
    });
    room.on(RoomEvent.Disconnected, () => {
      if (!leaving) handlers.current.onLost?.();
    });

    (async () => {
      try {
        await room.connect(url, token);
        await room.localParticipant.setMicrophoneEnabled(true);
        if (!leaving) handlers.current.onConnected?.();
      } catch {
        if (!leaving) handlers.current.onConnectFailed?.();
        void room.disconnect();
      }
    })();

    return () => {
      leaving = true;
      roomRef.current = null;
      setMuted(false);
      void room.disconnect();
    };
  }, [url, token, audioContainer]);

  const toggleMute = useCallback(async () => {
    const room = roomRef.current;
    if (!room) return;
    await room.localParticipant.setMicrophoneEnabled(muted);
    setMuted(!muted);
  }, [muted]);

  return { muted, toggleMute };
}
