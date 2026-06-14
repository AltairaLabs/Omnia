"use client";

import type { ReactNode } from "react";
import { X, Trash2 } from "lucide-react";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import { TierBadge } from "./tier-badge";
import { CategoryBadge } from "./category-badge";
import { cn } from "@/lib/utils";

interface MemoryGalaxyBubbleProps {
  point: GalaxyPoint;
  x: number; // anchor screen x within the galaxy container
  y: number; // anchor screen y within the galaxy container
  placement: "above" | "below";
  onClose: () => void;
  onDelete: (id: string) => void;
}

function fmtDate(d?: string): string {
  return d ? new Date(d).toLocaleDateString() : "—";
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate">{children}</span>
    </div>
  );
}

// A speech-bubble popup anchored at a point's screen position, growing out of
// the point. Positioned by the galaxy (which knows the live view transform).
export function MemoryGalaxyBubble({
  point,
  x,
  y,
  placement,
  onClose,
  onDelete,
}: Readonly<MemoryGalaxyBubbleProps>) {
  const above = placement === "above";
  return (
    <div className="pointer-events-none absolute z-30" style={{ left: x, top: y }}>
      <div
        className={cn(
          "pointer-events-auto w-72 -translate-x-1/2 rounded-lg border bg-popover text-popover-foreground shadow-xl",
          "animate-in fade-in-0 zoom-in-95 duration-150",
          above ? "-translate-y-full origin-bottom" : "origin-top",
        )}
        style={{ marginTop: above ? -10 : 10 }}
      >
        <div className="flex items-start gap-2 border-b p-3">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{point.title ?? "Memory"}</div>
            <div className="mt-1 flex flex-wrap gap-1">
              <TierBadge tier={point.tier} />
              <CategoryBadge category={point.category} />
            </div>
          </div>
          <button
            type="button"
            aria-label="Close"
            data-testid="bubble-close"
            onClick={onClose}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="max-h-48 space-y-2 overflow-auto p-3 text-xs">
          {point.preview && (
            <p className="whitespace-pre-wrap rounded bg-muted/50 p-2">{point.preview}</p>
          )}
          <Row label="Type">{point.type ?? "—"}</Row>
          <Row label="Confidence">{Math.round(point.confidence * 100)}%</Row>
          <Row label="User">{point.userRef ?? "—"}</Row>
          <Row label="Created">{fmtDate(point.observedAt)}</Row>
          <Row label="Expires">{fmtDate(point.expiresAt)}</Row>
        </div>

        <div className="border-t p-2">
          <button
            type="button"
            data-testid="bubble-delete"
            onClick={() => onDelete(point.id)}
            className="flex w-full items-center justify-center gap-2 rounded bg-destructive/10 py-1.5 text-xs font-medium text-destructive transition-colors hover:bg-destructive/20"
          >
            <Trash2 className="h-3.5 w-3.5" />
            Delete
          </button>
        </div>

        <div
          className={cn(
            "absolute left-1/2 h-2.5 w-2.5 -translate-x-1/2 rotate-45 border bg-popover",
            above ? "bottom-[-6px] border-l-0 border-t-0" : "top-[-6px] border-b-0 border-r-0",
          )}
        />
      </div>
    </div>
  );
}
