/**
 * ConsentBanner — toggles for privacy consent categories.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Shield } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { useConsent, useUpdateConsent } from "@/hooks/use-consent";
import { getCategoryLabel } from "./category-badge";

// Categories that require explicit user grant (PII)
const PII_CATEGORIES = [
  "memory:identity",
  "memory:location",
  "memory:health",
];

// Categories that are implicitly granted (non-PII)
const DEFAULT_CATEGORIES = [
  "memory:preferences",
  "memory:context",
  "memory:history",
];

const SKELETON_KEYS = ["sk-0", "sk-1", "sk-2", "sk-3", "sk-4", "sk-5"];

function LoadingSkeleton() {
  return (
    <Card data-testid="consent-banner" className="mb-4">
      <CardContent className="p-4">
        <Skeleton className="h-6 w-48 mb-3" />
        <div className="flex gap-6">
          {SKELETON_KEYS.map((key) => (
            <Skeleton key={key} className="h-8 w-32" />
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

export function ConsentBanner() {
  const { data: consent, isLoading } = useConsent();
  const updateConsent = useUpdateConsent();

  const handleToggle = (category: string, granted: boolean) => {
    if (granted) {
      updateConsent.mutate({ grants: [category] });
    } else {
      updateConsent.mutate({ revocations: [category] });
    }
  };

  if (isLoading) {
    return <LoadingSkeleton />;
  }

  const grants = new Set(consent?.grants ?? []);

  return (
    <Card data-testid="consent-banner" className="mb-4">
      <CardHeader className="pb-2 pt-3 px-4">
        <CardTitle className="text-sm font-medium flex items-center gap-2">
          <Shield className="h-4 w-4" />
          Privacy Consent
        </CardTitle>
      </CardHeader>
      <CardContent className="px-4 pb-3">
        <div className="flex flex-wrap gap-x-6 gap-y-2">
          {/* PII categories — user toggleable */}
          {PII_CATEGORIES.map((cat) => (
            <div key={cat} className="flex items-center gap-2">
              <Switch
                id={`consent-${cat}`}
                checked={grants.has(cat)}
                onCheckedChange={(checked) => handleToggle(cat, checked)}
                disabled={updateConsent.isPending}
                data-testid={`consent-toggle-${cat}`}
              />
              <Label htmlFor={`consent-${cat}`} className="text-sm cursor-pointer">
                {getCategoryLabel(cat)}
              </Label>
            </div>
          ))}
          {/* Non-PII categories — always on */}
          {DEFAULT_CATEGORIES.map((cat) => (
            <div key={cat} className="flex items-center gap-2">
              <Switch
                id={`consent-${cat}`}
                checked
                disabled
                data-testid={`consent-toggle-${cat}`}
              />
              <Label htmlFor={`consent-${cat}`} className="text-sm text-muted-foreground cursor-default">
                {getCategoryLabel(cat)} <span className="text-xs">(default)</span>
              </Label>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
