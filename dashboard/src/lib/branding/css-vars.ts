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
  category1: "--chart-1",
  category2: "--chart-2",
  category3: "--chart-3",
  category4: "--chart-4",
  category5: "--chart-5",
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
    vars["--font-sans"] = `${cfg.fonts.family}, ui-sans-serif, system-ui, sans-serif`;
  }
  return vars;
}
