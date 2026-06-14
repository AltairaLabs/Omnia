"use client";

import { createPortal } from "react-dom";
import type { ReactNode } from "react";
import { X, Trash2 } from "lucide-react";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import { TierBadge } from "./tier-badge";
import { CategoryBadge } from "./category-badge";
import { cn } from "@/lib/utils";

interface MemoryGalaxyBubbleProps {
  point: GalaxyPoint;
  left: number; // viewport x of the node (tail target)
  top: number; // viewport y of the node (tail target)
  placement: "above" | "below";
  tailOffset: number; // px from the card centre to the tail, so it points at the node when clamped
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

// A speech-bubble popup, portalled to <body> (so it's never clipped by the
// galaxy's overflow and is always clickable), anchored at a node's viewport
// position with a tail pointing back at the node.
export function MemoryGalaxyBubble({
  point,
  left,
  top,
  placement,
  tailOffset,
  onClose,
  onDelete,
}: Readonly<MemoryGalaxyBubbleProps>) {
  if (typeof document === "undefined") return null;
  const above = placement === "above";
  const tailLeft = `calc(50% + ${tailOffset}px)`;

  return createPortal(
    <div className="pointer-events-none fixed z-[60]" style={{ left, top }}>
      <div
        className={cn(
          "pointer-events-auto relative w-72 -translate-x-1/2 rounded-xl border bg-popover text-popover-foreground shadow-xl",
          "animate-in fade-in-0 zoom-in-95 duration-150",
          above && "-translate-y-full",
        )}
        style={{
          marginTop: above ? -12 : 12,
          transformOrigin: `${tailLeft} ${above ? "bottom" : "top"}`,
        }}
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

        {/* Tail: a rotated square half-out of the edge nearest the node. */}
        <div
          className={cn(
            "absolute h-3 w-3 -ml-1.5 rotate-45 border bg-popover",
            above ? "bottom-[-6px] border-l-0 border-t-0" : "top-[-6px] border-b-0 border-r-0",
          )}
          style={{ left: tailLeft }}
        />
      </div>
    </div>,
    document.body,
  );
}
