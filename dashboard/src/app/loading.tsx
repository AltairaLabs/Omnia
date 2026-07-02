"use client";

import { Loader2 } from "lucide-react";
import { useBrand } from "@/hooks/use-brand";

/** Brand-aware, token-styled route-level suspense fallback. */
export default function Loading() {
  const { brand } = useBrand();
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex min-h-[60vh] flex-col items-center justify-center gap-3 text-muted-foreground"
    >
      <Loader2 className="h-6 w-6 animate-spin text-primary" />
      <p className="text-sm">Loading {brand.productName}…</p>
    </div>
  );
}
