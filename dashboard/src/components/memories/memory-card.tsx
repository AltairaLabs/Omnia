/**
 * MemoryCard — compact collapsible card for the sidebar memory list.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { CategoryBadge } from "./category-badge";
import { TierBadge } from "./tier-badge";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { MemoryEntity } from "@/lib/data/types";

function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ago`;
}

function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + "...";
}

export function MemoryCard({ memory }: { memory: MemoryEntity }) {
  const [open, setOpen] = useState(false);
  const category = memory.metadata?.consent_category as string | undefined;
  const confidence = memory.confidence ?? 0;

  return (
    <Collapsible open={open} onOpenChange={setOpen} data-testid="memory-card">
      <Card className="mb-2">
        <CollapsibleTrigger asChild>
          <CardContent className="p-3 cursor-pointer hover:bg-muted/50 transition-colors">
            <div className="flex items-start gap-2">
              <div className="mt-0.5">
                {open ? (
                  <ChevronDown className="h-4 w-4 text-muted-foreground" />
                ) : (
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                )}
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm leading-snug">{truncate(memory.content, 100)}</p>
                <div className="flex items-center gap-2 mt-1.5">
                  <TierBadge tier={memory.tier} />
                  <CategoryBadge category={category} />
                  <div className="h-1.5 w-12 bg-muted rounded-full overflow-hidden">
                    <div
                      className="h-full bg-primary rounded-full"
                      style={{ width: `${Math.round(confidence * 100)}%` }}
                      data-testid="confidence-bar"
                    />
                  </div>
                  <span className="text-xs text-muted-foreground">
                    {formatRelativeTime(memory.createdAt)}
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="px-3 pb-3 pt-0 border-t">
            <p className="text-sm mt-2 whitespace-pre-wrap">{memory.content}</p>
            <dl className="mt-2 text-xs text-muted-foreground space-y-1">
              <div>
                <dt className="inline font-medium">Type:</dt>{" "}
                <dd className="inline">{memory.type}</dd>
              </div>
              <div>
                <dt className="inline font-medium">Confidence:</dt>{" "}
                <dd className="inline">{Math.round(confidence * 100)}%</dd>
              </div>
              {memory.sessionId && (
                <div>
                  <dt className="inline font-medium">Session:</dt>{" "}
                  <dd className="inline">{memory.sessionId}</dd>
                </div>
              )}
            </dl>
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  );
}
