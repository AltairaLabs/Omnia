"use client";

import { Lock } from "lucide-react";
import { useReadOnly } from "@/hooks";

/**
 * Banner displayed at the top of the page when the dashboard is in read-only mode.
 * Only renders when NEXT_PUBLIC_READ_ONLY_MODE=true.
 */
export function ReadOnlyBanner() {
  const { isReadOnly, message } = useReadOnly();

  if (!isReadOnly) {
    return null;
  }

  return (
    <div className="bg-muted border-b px-4 py-2">
      <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
        <Lock className="h-3.5 w-3.5" />
        <span>{message}</span>
      </div>
    </div>
  );
}
