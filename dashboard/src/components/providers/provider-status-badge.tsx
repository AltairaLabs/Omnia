"use client";

import { Badge } from "@/components/ui/badge";

interface ProviderStatusBadgeProps {
  phase?: string;
}

export function ProviderStatusBadge({ phase }: Readonly<ProviderStatusBadgeProps>) {
  if (!phase) return <Badge variant="outline">Unknown</Badge>;

  switch (phase) {
    case "Ready":
      return (
        <Badge className="bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/30">
          Ready
        </Badge>
      );
    case "Error":
      return (
        <Badge className="bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/30">
          Error
        </Badge>
      );
    default:
      return <Badge variant="outline">{phase}</Badge>;
  }
}
