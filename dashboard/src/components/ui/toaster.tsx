"use client";

import { X } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { cn } from "@/lib/utils";

/**
 * Renders the active toasts from the module-level toast store. Mount once near
 * the app root; any `toast()` / `useToast().toast()` call anywhere surfaces here.
 */
export function Toaster() {
  const { toasts, dismiss } = useToast();

  if (toasts.length === 0) return null;

  return (
    <div
      className="fixed bottom-4 right-4 z-[100] flex w-full max-w-sm flex-col gap-2"
      role="region"
      aria-label="Notifications"
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          className={cn(
            "relative rounded-md border p-4 pr-8 shadow-lg",
            t.variant === "destructive"
              ? "border-destructive bg-destructive text-destructive-foreground"
              : "bg-card text-card-foreground"
          )}
        >
          <button
            type="button"
            aria-label="Dismiss notification"
            onClick={() => dismiss(t.id)}
            className="absolute right-2 top-2 opacity-70 transition-opacity hover:opacity-100"
          >
            <X className="h-4 w-4" />
          </button>
          {t.title && <div className="text-sm font-medium">{t.title}</div>}
          {t.description && <div className="text-sm opacity-90">{t.description}</div>}
        </div>
      ))}
    </div>
  );
}
