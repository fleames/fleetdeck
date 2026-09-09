"use client";

import { useEffect, useRef } from "react";
import { realtimeWsURL } from "@/lib/api";

/** Single multiplexed WebSocket; invokes onMessage for each event. */
export function useRealtime(onMessage: (type: string, data: unknown) => void, enabled = true) {
  const cb = useRef(onMessage);

  useEffect(() => {
    cb.current = onMessage;
  }, [onMessage]);

  useEffect(() => {
    if (!enabled) return;
    const wsURL = realtimeWsURL();
    let ws: WebSocket | null = null;
    let closed = false;
    let retry = 0;
    let timer: number | undefined;

    function connect() {
      ws = new WebSocket(wsURL);
      ws.onopen = () => {
        retry = 0;
      };
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(String(ev.data)) as { type: string; data: unknown };
          cb.current(msg.type, msg.data);
        } catch {
          // ignore malformed
        }
      };
      ws.onclose = () => {
        if (closed) return;
        const delay = Math.min(15000, 1000 * 2 ** retry);
        retry += 1;
        timer = window.setTimeout(connect, delay);
      };
    }

    connect();
    return () => {
      closed = true;
      if (timer) window.clearTimeout(timer);
      ws?.close();
    };
  }, [enabled]);
}
