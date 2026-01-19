"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { Globe2 } from "lucide-react";

interface SharedBadgeProps {
  className?: string;
}

/**
 * Badge indicating a resource is shared across all workspaces.
 * Shared resources are read-only and come from the system namespace.
 */
export function SharedBadge({ className }: Readonly<SharedBadgeProps>) {
  return (
    <Badge
      variant="outline"
      className={cn(
        "text-xs bg-blue-500/15 text-blue-700 dark:text-blue-400 border-blue-500/20",
        className
      )}
      data-testid="shared-badge"
    >
      <Globe2 className="h-3 w-3 mr-1" />
      Shared
    </Badge>
  );
}
