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

function cssBlock(selector: string, vars: Record<string, string>): string {
  const decls = Object.entries(vars)
    .map(([key, value]) => `  ${key}: ${value};`)
    .join("\n");
  return `${selector} {\n${decls}\n}`;
}

/**
 * Render the scoped variable overrides for a brand: `:root` for the light /
 * shared tokens, and `.dark` for any dark-mode-specific overrides (surfaces).
 * The `.dark` block is emitted after `:root` so it wins in dark mode.
 */
function renderCss(brand: BrandConfig): string {
  const parts = [cssBlock(":root", brandConfigToCssVars(brand))];
  if (brand.colorsDark && Object.keys(brand.colorsDark).length > 0) {
    // Only the dark colors — fonts/customCss are theme-independent.
    const darkVars = brandConfigToCssVars({ ...brand, colors: brand.colorsDark, fonts: undefined });
    parts.push(cssBlock(".dark", darkVars));
  }
  // customCss is always confined to :root — token overrides, not selectors.
  if (brand.customCss) parts.push(`:root {\n  ${brand.customCss}\n}`);
  return parts.join("\n");
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
      {/*
        Load the brand's webfont stylesheet so a custom `fonts.family` actually
        renders. `fonts.family` re-points `--font-sans` (see css-vars.ts), but
        the family only resolves if its @font-face is available — this fetches
        it. `fonts.url` must be a CSS stylesheet URL (e.g. a Google Fonts href);
        the brand's font host must be allowed by the dashboard CSP.
      */}
      {brand.fonts?.url && (
        <link rel="stylesheet" href={brand.fonts.url} data-brand-font="" />
      )}
      <style id="brand-vars" dangerouslySetInnerHTML={{ __html: renderCss(brand) }} />
      {children}
    </BrandContext.Provider>
  );
}
