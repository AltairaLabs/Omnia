"use client";

import { useSyncExternalStore } from "react";

// Tailwind `lg` breakpoint: collapse the sidebar below 1024px.
const NARROW_QUERY = "(max-width: 1023px)";

function subscribe(callback: () => void): () => void {
  const mql = window.matchMedia(NARROW_QUERY);
  mql.addEventListener("change", callback);
  return () => mql.removeEventListener("change", callback);
}

function getSnapshot(): boolean {
  return window.matchMedia(NARROW_QUERY).matches;
}

function getServerSnapshot(): boolean {
  return false;
}

/**
 * True when the viewport is narrower than the `lg` breakpoint.
 * SSR-safe: returns false on the server, syncs to matchMedia on the client.
 */
export function useIsNarrow(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
