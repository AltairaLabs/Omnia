/**
 * White-label theme visual-regression harness (#1690).
 *
 * For each brand preset × color mode, loads the /dev/theme kitchen-sink, applies
 * the preset via the dev switcher, asserts the injected design-token CSS
 * variables actually changed (a deterministic, font-independent gate), and
 * captures a screenshot artifact for eyeballing.
 *
 * Opt-in project (not part of the default CI e2e run — needs a demo-mode server):
 *   npm run test:e2e:theme
 *
 * The dev webServer sets NEXT_PUBLIC_DEMO_MODE=true, which makes /dev/theme and
 * the preset switcher reachable. Screenshots land in e2e/screenshots/output/.
 */

import { test, expect, type Page } from "@playwright/test";

interface Preset {
  name: string;
  label: RegExp;
  /** Expected --primary hex once applied; null = theme default (no override). */
  primary: string | null;
}

const PRESETS: Preset[] = [
  { name: "omnia", label: /Omnia/i, primary: null },
  { name: "acme", label: /Acme Cloud/i, primary: "#EA580C" },
  { name: "nebula", label: /Nebula/i, primary: "#7C3AED" },
];

const MODES = ["light", "dark"] as const;

async function applyMode(page: Page, mode: string): Promise<void> {
  // next-themes reads the "theme" localStorage key before first paint.
  await page.addInitScript((m) => window.localStorage.setItem("theme", m), mode);
}

async function selectPreset(page: Page, label: RegExp): Promise<void> {
  await page.getByTestId("brand-preset-switcher").first().click();
  await page.getByRole("menuitem", { name: label }).click();
}

test.describe("white-label theme presets", () => {
  for (const mode of MODES) {
    for (const preset of PRESETS) {
      test(`/dev/theme renders ${preset.name} in ${mode} mode`, async ({ page }) => {
        await applyMode(page, mode);
        await page.goto("/dev/theme");
        await expect(page.getByTestId("theme-preview")).toBeVisible();

        await selectPreset(page, preset.label);

        if (preset.primary) {
          const primary = await page.evaluate(() =>
            getComputedStyle(document.documentElement)
              .getPropertyValue("--primary")
              .trim(),
          );
          expect(primary.toLowerCase()).toContain(preset.primary.toLowerCase());
        }

        await page.screenshot({
          path: `e2e/screenshots/output/theme-${preset.name}-${mode}.png`,
          fullPage: true,
        });
      });
    }
  }
});
