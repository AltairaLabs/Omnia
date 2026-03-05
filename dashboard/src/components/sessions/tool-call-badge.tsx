"use client";

import { Badge } from "@/components/ui/badge";
import { CheckCircle2, XCircle, Loader2 } from "lucide-react";
import type { ToolCall } from "@/types/session";

interface ToolCallBadgeProps {
  readonly status: ToolCall["status"];
}

export function ToolCallBadge({ status }: ToolCallBadgeProps) {
  switch (status) {
    case "success":
      return (
        <Badge variant="secondary" className="gap-1" data-testid="tool-call-badge">
          <CheckCircle2 className="h-3 w-3 text-green-500" />
          Success
        </Badge>
      );
    case "error":
      return (
        <Badge variant="destructive" className="gap-1" data-testid="tool-call-badge">
          <XCircle className="h-3 w-3" />
          Error
        </Badge>
      );
    case "pending":
      return (
        <Badge variant="outline" className="gap-1" data-testid="tool-call-badge">
          <Loader2 className="h-3 w-3 animate-spin" />
          Pending
        </Badge>
      );
  }
}
