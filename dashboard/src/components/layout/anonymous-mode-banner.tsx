"use client";

import { ShieldAlert } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useDemoMode } from "@/hooks/use-runtime-config";

/**
 * Banner warning that the dashboard is running unauthenticated.
 *
 * Renders when the current user's provider is "anonymous" — which
 * happens either because OMNIA_AUTH_MODE=anonymous was explicitly
 * opted-in to, or because a misconfiguration slipped past the
 * chart/runtime guardrails. Intentionally red and persistent so it's
 * hard to forget the deployment is unsecured.
 *
 * Suppressed when NEXT_PUBLIC_DEMO_MODE=true — demo deployments
 * already render the amber DemoModeBanner, which carries the "this is
 * a sandbox" meaning. A second red warning layered on top is
 * redundant and makes marketing/docs screenshots look alarming. The
 * Helm chart and runtime boot guard still enforce the real "no silent
 * anonymous prod" contract; this banner is a UI nicety, not the gate.
 */
export function AnonymousModeBanner() {
  const { user } = useAuth();
  const { isDemoMode } = useDemoMode();

  if (user.provider !== "anonymous") return null;
  if (isDemoMode) return null;

  return (
    <div
      role="alert"
      className="bg-red-500/15 border-b border-red-500/40 px-4 py-2"
    >
      <div className="flex items-center justify-center gap-2 text-sm font-medium text-red-600 dark:text-red-400">
        <ShieldAlert className="h-3.5 w-3.5" />
        <span>
          Anonymous access — authentication is disabled. Do not use this configuration outside an isolated dev environment.
        </span>
      </div>
    </div>
  );
}
