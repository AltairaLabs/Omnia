"use client";

import { useCallback, useSyncExternalStore } from "react";

// A list/grid view-mode preference persisted to localStorage. Generic over the
// page's own value union so each page keeps its existing vocabulary
// (cards/table, grid/table, grid/list). Uses useSyncExternalStore so it is
// hydration-safe: the server and first client render use the default, then the
// client syncs to the stored value. Mirrors use-persisted-node-layout's
// localStorage-as-view-preference pattern.

// Module-level listeners notify every hook instance when any value is written
// from this document (localStorage's native `storage` event only fires across
// documents/tabs).
const listeners = new Set<() => void>();

function notify(): void {
  listeners.forEach((l) => l());
}

function subscribe(callback: () => void): () => void {
  listeners.add(callback);
  window.addEventListener("storage", callback);
  return () => {
    listeners.delete(callback);
    window.removeEventListener("storage", callback);
  };
}

function read(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function usePersistedViewMode<T extends string>(
  storageKey: string,
  defaultValue: T,
): [T, (value: T) => void] {
  const getSnapshot = useCallback(
    (): T => (read(storageKey) as T | null) ?? defaultValue,
    [storageKey, defaultValue],
  );
  const getServerSnapshot = useCallback((): T => defaultValue, [defaultValue]);

  const mode = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setMode = useCallback(
    (value: T) => {
      try {
        window.localStorage.setItem(storageKey, value);
      } catch {
        /* quota exceeded / storage disabled — ignore */
      }
      notify();
    },
    [storageKey],
  );

  return [mode, setMode];
}
