"use client";

import { useState } from "react";
import { Palette, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useBrand } from "@/hooks/use-brand";
import { useDevMode, useDemoMode } from "@/hooks/core";
import { BRAND_PRESETS } from "@/lib/branding/presets";
import type { BrandConfig } from "@/lib/branding/types";

// Explicit list (name + label + config) so there are no unreachable fallback
// branches for missing presets/labels — every entry is known-valid.
const PRESET_OPTIONS: { name: string; label: string; config: BrandConfig }[] = [
  { name: "omnia", label: "Omnia (default)", config: BRAND_PRESETS.omnia },
  { name: "acme", label: "Acme Cloud", config: BRAND_PRESETS.acme },
  { name: "nebula", label: "Nebula", config: BRAND_PRESETS.nebula },
];

/**
 * Dev/demo-only brand preset switcher. Flips the in-memory brand override so a
 * developer can preview each preset against the live UI without a cluster. It
 * renders nothing in a real deployment — one brand per deployment comes from
 * the ConfigMap, so this control must never appear there.
 */
export function BrandPresetSwitcher() {
  const { isDevMode, loading: devLoading } = useDevMode();
  const { isDemoMode, loading: demoLoading } = useDemoMode();
  const { setBrandOverride } = useBrand();
  const [active, setActive] = useState<string>("omnia");

  if (devLoading || demoLoading) return null;
  if (!isDevMode && !isDemoMode) return null;

  const select = (config: BrandConfig, name: string) => {
    setActive(name);
    setBrandOverride(config);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Switch brand preset"
          data-testid="brand-preset-switcher"
        >
          <Palette className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Brand preset (dev)</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {PRESET_OPTIONS.map((p) => (
          <DropdownMenuItem key={p.name} onClick={() => select(p.config, p.name)}>
            <Check
              className={`mr-2 h-4 w-4 ${active === p.name ? "opacity-100" : "opacity-0"}`}
            />
            {p.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
