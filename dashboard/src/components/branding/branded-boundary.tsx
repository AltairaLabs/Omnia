"use client";

import type { ReactNode } from "react";
import { useBrand } from "@/hooks/use-brand";

export interface BrandedBoundaryProps {
  /** Short heading, e.g. "Page not found". */
  title: string;
  /** Supporting copy under the heading. */
  description: string;
  /** Optional status code shown prominently, e.g. "404" / "500". */
  code?: string;
  /** Optional action area (buttons / links). */
  action?: ReactNode;
}

/**
 * Token-styled shell for the app-level not-found / error / global-error pages.
 *
 * Renders the active white-label brand name and uses design-token color
 * classes only, so error boundaries re-theme with the SI partner's brand
 * instead of falling back to unbranded Next.js defaults.
 */
export function BrandedBoundary({
  title,
  description,
  code,
  action,
}: Readonly<BrandedBoundaryProps>) {
  const { brand } = useBrand();
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 p-8 text-center">
      <p className="text-sm font-medium text-muted-foreground">{brand.productName}</p>
      {code && <p className="text-5xl font-bold text-primary">{code}</p>}
      <h1 className="text-2xl font-semibold text-foreground">{title}</h1>
      <p className="max-w-md text-muted-foreground">{description}</p>
      {action && <div className="flex gap-3">{action}</div>}
    </div>
  );
}
