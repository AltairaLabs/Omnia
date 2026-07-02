/**
 * Brand preset registry.
 *
 * Named BrandConfig presets used by the dev/demo brand preset switcher and the
 * /dev/theme preview route, and resolvable server-side in demo mode via
 * NEXT_PUBLIC_BRAND_PRESET. Presets are the cluster-free way to exercise the
 * white-label token contract locally — one brand per real deployment still
 * comes from the ConfigMap, never from these.
 */

import type { BrandConfig } from "../types";
import omnia from "./omnia.json";
import acme from "./acme.json";
import nebula from "./nebula.json";

/** All built-in presets, keyed by their switcher/env name. */
export const BRAND_PRESETS: Record<string, BrandConfig> = {
  omnia: omnia as BrandConfig,
  acme: acme as BrandConfig,
  nebula: nebula as BrandConfig,
};

/** Preset names in display order. */
export const PRESET_NAMES: string[] = ["omnia", "acme", "nebula"];

/** Resolve a preset by name; undefined for an unknown or missing name. */
export function getBrandPreset(name: string | undefined): BrandConfig | undefined {
  if (!name) return undefined;
  return BRAND_PRESETS[name];
}
