"use client";

import { cn } from "@/lib/utils";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";

export interface Facet {
  key: string;
  label: string;
  color: string;
  count: number;
  description?: string;
}

interface FacetRailProps {
  facets: Facet[];
  hidden: Set<string>;
  onToggle: (key: string) => void;
}

// Colored show/hide pills for the active dimension (tiers or categories).
export function FacetRail({ facets, hidden, onToggle }: Readonly<FacetRailProps>) {
  return (
    <TooltipProvider>
      <div className="flex flex-wrap items-center gap-2" data-testid="facet-rail">
        {facets.map((f) => {
          const isHidden = hidden.has(f.key);
          return (
            <Tooltip key={f.key}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  data-testid={`facet-chip-${f.key}`}
                  aria-pressed={!isHidden}
                  onClick={() => onToggle(f.key)}
                  className={cn(
                    "flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium transition-opacity",
                    isHidden && "opacity-40",
                  )}
                >
                  <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: f.color }} />
                  <span>{f.label}</span>
                  <span className="text-muted-foreground" data-testid={`facet-count-${f.key}`}>
                    {f.count}
                  </span>
                </button>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-xs">
                {f.description ?? f.label}
              </TooltipContent>
            </Tooltip>
          );
        })}
      </div>
    </TooltipProvider>
  );
}
