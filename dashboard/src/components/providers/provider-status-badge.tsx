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
        <Badge className="bg-success/15 text-success border-success/30">
          Ready
        </Badge>
      );
    case "Error":
      return (
        <Badge className="bg-destructive/15 text-destructive border-destructive/30">
          Error
        </Badge>
      );
    default:
      return <Badge variant="outline">{phase}</Badge>;
  }
}
