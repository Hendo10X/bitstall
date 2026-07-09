"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export interface PollState<T> {
  data: T | null;
  /** true when the most recent request failed (daemon offline, etc.) */
  error: boolean;
  /** trigger an immediate out-of-band refresh (e.g. after a POST) */
  refresh: () => Promise<void>;
}

/**
 * Tiny polling hook: runs `fn` immediately and then every `intervalMs`.
 * Keeps the last good data on failure and never throws so the daemon
 * going away degrades the UI instead of crashing it.
 */
export function usePoll<T>(
  fn: () => Promise<T>,
  intervalMs: number,
  enabled: boolean = true
): PollState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState(false);

  const fnRef = useRef(fn);
  fnRef.current = fn;
  const aliveRef = useRef(true);

  const tick = useCallback(async () => {
    try {
      const result = await fnRef.current();
      if (aliveRef.current) {
        setData(result);
        setError(false);
      }
    } catch {
      if (aliveRef.current) setError(true);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    aliveRef.current = true;
    void tick();
    const id = setInterval(() => void tick(), intervalMs);
    return () => {
      aliveRef.current = false;
      clearInterval(id);
    };
  }, [intervalMs, enabled, tick]);

  return { data, error, refresh: tick };
}
