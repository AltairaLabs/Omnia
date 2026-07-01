"use client";

import { createContext, useEffect, useMemo, useState } from "react";
import { getRuntimeConfig } from "@/lib/config";
import { brandConfigToCssVars } from "@/lib/branding/css-vars";
import { type BrandConfig, OMNIA_BRAND } from "@/lib/branding/types";

export interface BrandContextValue {
  brand: BrandConfig;
  /** Dev/demo-only in-memory override (used by the brand preset switcher). */
  setBrandOverride: (brand: BrandConfig | null) => void;
}

export const BrandContext = createContext<BrandContextValue>({
  brand: OMNIA_BRAND,
  setBrandOverride: () => {},
});

/** Render the `:root` variable overrides (and optional customCss) for a brand. */
function renderCss(brand: BrandConfig): string {
  const vars = brandConfigToCssVars(brand);
  const decls = Object.entries(vars)
    .map(([key, value]) => `  ${key}: ${value};`)
    .join("\n");
  const base = `:root {\n${decls}\n}`;
  // customCss is always confined to :root — token overrides, not selectors.
  return brand.customCss ? `${base}\n:root {\n  ${brand.customCss}\n}` : base;
}

export function BrandProvider({ children }: Readonly<{ children: React.ReactNode }>) {
  const [resolved, setResolved] = useState<BrandConfig>(OMNIA_BRAND);
  const [override, setOverride] = useState<BrandConfig | null>(null);

  useEffect(() => {
    let active = true;
    getRuntimeConfig().then((config) => {
      if (active && config.brand) setResolved(config.brand);
    });
    return () => {
      active = false;
    };
  }, []);

  const brand = override ?? resolved;
  const value = useMemo<BrandContextValue>(
    () => ({ brand, setBrandOverride: setOverride }),
    [brand],
  );

  return (
    <BrandContext.Provider value={value}>
      <style id="brand-vars" dangerouslySetInnerHTML={{ __html: renderCss(brand) }} />
      {children}
    </BrandContext.Provider>
  );
}
