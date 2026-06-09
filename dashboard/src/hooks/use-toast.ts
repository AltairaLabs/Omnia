"use client";

import { useEffect, useState } from "react";

export interface ToastOptions {
  title?: string;
  description?: string;
  variant?: "default" | "destructive";
  /** Auto-dismiss after this many ms. 0 disables auto-dismiss. Default 5000. */
  duration?: number;
}

export interface Toast extends ToastOptions {
  id: string;
}

const DEFAULT_DURATION = 5000;

// Module-level store so toasts raised anywhere surface in the single <Toaster>,
// without threading a context provider through the tree (shadcn-style).
let counter = 0;
let store: Toast[] = [];
const listeners = new Set<(toasts: Toast[]) => void>();

function emit() {
  for (const listener of listeners) listener(store);
}

/** Remove a toast by id. */
export function dismissToast(id: string): void {
  store = store.filter((t) => t.id !== id);
  emit();
}

/**
 * Raise a toast. Returns the toast id (so callers can dismiss early).
 * Stable module-level function — safe to call outside React.
 */
export function toast(options: ToastOptions): string {
  counter += 1;
  const id = `toast-${counter}`;
  store = [...store, { id, ...options }];
  emit();

  const duration = options.duration ?? DEFAULT_DURATION;
  if (duration > 0) {
    setTimeout(() => dismissToast(id), duration);
  }
  return id;
}

/**
 * Subscribe to the toast store. Returns the live toast list plus the (stable)
 * `toast` and `dismiss` helpers. The API is backwards-compatible with the
 * previous placeholder hook (`{ toast }`); existing call sites are unchanged.
 */
export function useToast() {
  const [toasts, setToasts] = useState<Toast[]>(store);

  useEffect(() => {
    listeners.add(setToasts);
    return () => {
      listeners.delete(setToasts);
    };
  }, []);

  return { toast, toasts, dismiss: dismissToast };
}
