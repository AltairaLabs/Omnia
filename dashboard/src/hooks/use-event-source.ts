"use client";

import { useState, useEffect, useRef } from "react";

export interface UseEventSourceOptions {
  /** Whether the connection is enabled (default: true). */
  enabled?: boolean;
}

export interface UseEventSourceResult<T> {
  /** Latest parsed data from the stream. */
  data: T | null;
  /** Whether the EventSource is currently connected. */
  connected: boolean;
}

/**
 * Generic hook for consuming a Server-Sent Events (SSE) endpoint.
 *
 * Connects to the given URL when enabled, parses incoming `data:` frames as JSON,
 * and exposes the latest snapshot. Cleans up on unmount or when disabled.
 *
 * @param url - The SSE endpoint URL, or null to disable.
 * @param options - Optional configuration.
 */
export function useEventSource<T>(
  url: string | null,
  options?: UseEventSourceOptions
): UseEventSourceResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  const enabled = options?.enabled !== false;

  useEffect(() => {
    if (!url || !enabled) {
      // Close any existing connection without calling setState in the effect body.
      // The cleanup function below handles state reset.
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
      return;
    }

    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => setConnected(true);

    es.onmessage = (event: MessageEvent) => {
      try {
        setData(JSON.parse(event.data) as T);
      } catch {
        // Ignore malformed frames
      }
    };

    es.onerror = () => {
      setConnected(false);
      // Don't close — EventSource will automatically retry
    };

    return () => {
      es.close();
      esRef.current = null;
      setConnected(false);
      setData(null);
    };
  }, [url, enabled]);

  return { data, connected };
}
