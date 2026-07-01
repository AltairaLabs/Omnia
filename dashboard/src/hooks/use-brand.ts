"use client";

import { useContext } from "react";
import { BrandContext, type BrandContextValue } from "@/components/branding/brand-provider";

/** Access the active white-label brand config and (dev-only) override setter. */
export function useBrand(): BrandContextValue {
  return useContext(BrandContext);
}
