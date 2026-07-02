"use client";

import { FlaskConical } from "lucide-react";
import { useDemoMode } from "@/hooks/core";

/**
 * Banner displayed at the top of the page when the dashboard is in demo mode.
 * Only renders when NEXT_PUBLIC_DEMO_MODE=true (read at runtime).
 */
export function DemoModeBanner() {
  const { isDemoMode, loading } = useDemoMode();

  // Don't show anything while loading or if not in demo mode
  if (loading || !isDemoMode) {
    return null;
  }

  return (
    <div className="bg-warning/10 border-b border-warning/20 px-4 py-2">
      <div className="flex items-center justify-center gap-2 text-sm text-warning">
        <FlaskConical className="h-3.5 w-3.5" />
        <span>
          Demo Mode - Displaying sample data. Connect to a cluster to see real agents.
        </span>
      </div>
    </div>
  );
}
