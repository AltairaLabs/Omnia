import type { BrandColorTokens, BrandConfig } from "./types";

/**
 * Allowlist mapping brand color keys → design-token CSS variables. Any key not
 * present here is ignored, so a brand config can never inject an arbitrary CSS
 * variable.
 */
const COLOR_KEY_TO_VAR: Record<keyof BrandColorTokens, string> = {
  primary: "--primary",
  accent: "--accent",
  sidebar: "--sidebar",
  background: "--background",
  foreground: "--foreground",
  card: "--card",
  muted: "--muted",
  mutedForeground: "--muted-foreground",
  border: "--border",
  chart1: "--chart-1",
  chart2: "--chart-2",
  chart3: "--chart-3",
  chart4: "--chart-4",
  chart5: "--chart-5",
  category1: "--category-1",
  category2: "--category-2",
  category3: "--category-3",
  category4: "--category-4",
  category5: "--category-5",
  category6: "--category-6",
  category7: "--category-7",
  category8: "--category-8",
  success: "--success",
  warning: "--warning",
  info: "--info",
};

/**
 * Convert a BrandConfig into the `:root` CSS-variable overrides it implies.
 * Only allowlisted, non-empty color keys and an optional font family are
 * emitted.
 */
export function brandConfigToCssVars(cfg: BrandConfig): Record<string, string> {
  const vars: Record<string, string> = {};
  const colors = cfg.colors ?? {};
  for (const key of Object.keys(COLOR_KEY_TO_VAR) as (keyof BrandColorTokens)[]) {
    const value = colors[key];
    if (typeof value === "string" && value.length > 0) {
      vars[COLOR_KEY_TO_VAR[key]] = value;
    }
  }
  if (cfg.fonts?.family) {
    // Consumed by globals.css `--font-sans`, which falls back to the bundled
    // Inter (`@theme inline` bakes the token into the utility, so a plain
    // `--font-sans` override would be a no-op — this indirection is what makes
    // the brand font actually apply at runtime). Emit only the family so the
    // global fallback stack survives if the webfont fails to load.
    vars["--brand-font-sans"] = cfg.fonts.family;
  }
  return vars;
}
