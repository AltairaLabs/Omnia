"use client";

import { FlaskConical } from "lucide-react";
import { isDemoMode } from "@/lib/api/client";

/**
 * Banner displayed at the top of the page when the dashboard is in demo mode.
 * Only renders when NEXT_PUBLIC_DEMO_MODE=true.
 */
export function DemoModeBanner() {
  if (!isDemoMode) {
    return null;
  }

  return (
    <div className="bg-amber-500/10 border-b border-amber-500/20 px-4 py-2">
      <div className="flex items-center justify-center gap-2 text-sm text-amber-600 dark:text-amber-400">
        <FlaskConical className="h-3.5 w-3.5" />
        <span>
          Demo Mode - Displaying sample data. Connect to a cluster to see real agents.
        </span>
      </div>
    </div>
  );
}
