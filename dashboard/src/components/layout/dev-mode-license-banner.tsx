"use client";

import { ShieldAlert } from "lucide-react";
import { useDevMode } from "@/hooks/core";
import { useBrand } from "@/hooks/use-brand";
import { OMNIA_BRAND } from "@/lib/branding/types";

/**
 * Banner displayed when the instance is running with a development license.
 * Detects dev mode via NEXT_PUBLIC_DEV_MODE runtime config (set by Helm when devMode=true).
 */
export function DevModeLicenseBanner() {
  const { isDevMode, loading } = useDevMode();
  const { brand } = useBrand();

  if (loading || !isDevMode) {
    return null;
  }

  const licenseUrl = brand.links?.upgradeUrl ?? OMNIA_BRAND.links?.upgradeUrl;

  return (
    <div className="bg-warning/10 border-b border-warning/20 px-4 py-2">
      <div className="flex items-center justify-center gap-2 text-sm text-warning">
        <ShieldAlert className="h-3.5 w-3.5" />
        <span>
          <strong>Development License</strong> — This instance is using a
          development license not intended for production workloads. Obtain a
          valid license from{" "}
          <a
            href={licenseUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:no-underline"
          >
            {brand.productName}
          </a>
        </span>
      </div>
    </div>
  );
}
