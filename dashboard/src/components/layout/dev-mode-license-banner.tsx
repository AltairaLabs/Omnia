"use client";

import { ShieldAlert } from "lucide-react";
import { useDevMode } from "@/hooks/use-runtime-config";

/**
 * Banner displayed when the instance is running with a development license.
 * Detects dev mode via NEXT_PUBLIC_DEV_MODE runtime config (set by Helm when devMode=true).
 */
export function DevModeLicenseBanner() {
  const { isDevMode, loading } = useDevMode();

  if (loading || !isDevMode) {
    return null;
  }

  return (
    <div className="bg-orange-500/10 border-b border-orange-500/20 px-4 py-2">
      <div className="flex items-center justify-center gap-2 text-sm text-orange-600 dark:text-orange-400">
        <ShieldAlert className="h-3.5 w-3.5" />
        <span>
          <strong>Development License</strong> — This instance is using a
          development license not intended for production workloads. Obtain a
          valid license at{" "}
          <a
            href="https://altairalabs.ai/licensing"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:no-underline"
          >
            altairalabs.ai/licensing
          </a>
        </span>
      </div>
    </div>
  );
}
