"use client";

import { useCallback } from "react";

interface ToastOptions {
  title?: string;
  description?: string;
  variant?: "default" | "destructive";
}

/**
 * Simple toast hook placeholder.
 * Can be replaced with a proper toast library later.
 */
export function useToast() {
  const toast = useCallback((_options: ToastOptions) => {
    // Placeholder - replace with actual toast implementation
    // e.g., sonner, react-hot-toast, or shadcn toast
  }, []);

  return { toast };
}
