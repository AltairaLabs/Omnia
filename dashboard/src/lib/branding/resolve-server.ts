/**
 * Server-side brand resolution.
 *
 * Reads NEXT_PUBLIC_BRAND_* env vars (sourced from Helm/ConfigMap) into a
 * BrandConfig, and enforces the whiteLabel license entitlement: an unentitled
 * license always collapses to the Omnia default, so open-core cannot
 * white-label by setting env vars alone. Imported only by server code
 * (/api/config and generateMetadata).
 */

import { type BrandConfig, OMNIA_BRAND } from "./types";
import { getBrandPreset } from "./presets";
import type { License } from "@/types/license";

/**
 * Resolve the brand, optionally honoring a named preset. Presets are a
 * dev/demo convenience (NEXT_PUBLIC_BRAND_PRESET) and are only consulted when
 * `allowPreset` is set — a real deployment always resolves from the full
 * NEXT_PUBLIC_BRAND_* env set. Falls back to env-based resolution for an
 * unknown or unset preset.
 */
export function resolveBrand(
  env: Record<string, string | undefined>,
  opts: { allowPreset: boolean },
): BrandConfig {
  if (opts.allowPreset) {
    const preset = getBrandPreset(env.NEXT_PUBLIC_BRAND_PRESET);
    if (preset) return preset;
  }
  return resolveBrandFromEnv(env);
}

export function resolveBrandFromEnv(env: Record<string, string | undefined>): BrandConfig {
  const name = env.NEXT_PUBLIC_BRAND_PRODUCT_NAME;
  if (!name) return OMNIA_BRAND;
  return {
    productName: name,
    logo: {
      light: env.NEXT_PUBLIC_BRAND_LOGO_LIGHT || OMNIA_BRAND.logo.light,
      dark: env.NEXT_PUBLIC_BRAND_LOGO_DARK || OMNIA_BRAND.logo.dark,
    },
    favicon: env.NEXT_PUBLIC_BRAND_FAVICON || OMNIA_BRAND.favicon,
    colors: {
      primary: env.NEXT_PUBLIC_BRAND_COLOR_PRIMARY,
      accent: env.NEXT_PUBLIC_BRAND_COLOR_ACCENT,
      sidebar: env.NEXT_PUBLIC_BRAND_COLOR_SIDEBAR,
    },
    fonts: {
      family: env.NEXT_PUBLIC_BRAND_FONT_FAMILY,
      url: env.NEXT_PUBLIC_BRAND_FONT_URL,
    },
    links: {
      docsBaseUrl: env.NEXT_PUBLIC_BRAND_DOCS_URL || OMNIA_BRAND.links?.docsBaseUrl,
      support: env.NEXT_PUBLIC_BRAND_SUPPORT || OMNIA_BRAND.links?.support,
      sales: env.NEXT_PUBLIC_BRAND_SALES || OMNIA_BRAND.links?.sales,
      upgradeUrl: env.NEXT_PUBLIC_BRAND_UPGRADE_URL || OMNIA_BRAND.links?.upgradeUrl,
    },
    copy: {
      loginTagline: env.NEXT_PUBLIC_BRAND_LOGIN_TAGLINE,
      signupTagline: env.NEXT_PUBLIC_BRAND_SIGNUP_TAGLINE,
    },
    customCss: env.NEXT_PUBLIC_BRAND_CUSTOM_CSS,
  };
}

export function applyEntitlement(brand: BrandConfig, license: License): BrandConfig {
  return license.features.whiteLabel ? brand : OMNIA_BRAND;
}
