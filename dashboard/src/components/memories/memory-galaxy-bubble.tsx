"use client";

import { createPortal } from "react-dom";
import type { ReactNode } from "react";
import { X, Trash2, ChevronLeft, ChevronRight } from "lucide-react";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import { TierBadge } from "./tier-badge";
import { CategoryBadge } from "./category-badge";
import { cn } from "@/lib/utils";

interface MemoryGalaxyBubbleProps {
  // The memories stacked at the clicked location, ordered for browsing. One
  // popup pages through them (prev/next) instead of opening a window per point.
  stack: GalaxyPoint[];
  index: number; // which memory in the stack is currently shown
  left: number; // viewport x of the node (tail target)
  top: number; // viewport y of the node (tail target)
  placement: "above" | "below";
  tailOffset: number; // px from the card centre to the tail, so it points at the node when clamped
  onPrev: () => void;
  onNext: () => void;
  onClose: () => void;
  onDelete: (id: string) => void;
}

const GAP = 10; // gap between the card edge and the node, bridged by the tail

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

// A speech-bubble popup, portalled to <body> so it's never clipped by the
// galaxy's overflow and is always clickable. A triangle tail bridges the gap to
// the node and points at it. When several memories share the clicked point the
// footer shows a prev/next carousel so all of them are reachable from one popup.
export function MemoryGalaxyBubble({
  stack,
  index,
  left,
  top,
  placement,
  tailOffset,
  onPrev,
  onNext,
  onClose,
  onDelete,
}: Readonly<MemoryGalaxyBubbleProps>) {
  if (typeof document === "undefined") return null;
  const point = stack[index];
  if (!point) return null;
  const above = placement === "above";
  const tailLeft = `calc(50% + ${tailOffset}px)`;
  const many = stack.length > 1;
  const atStart = index <= 0;
  const atEnd = index >= stack.length - 1;

  return createPortal(
    <div
      className={cn(
        "fixed z-[60] w-72 -translate-x-1/2 rounded-xl border bg-popover text-popover-foreground shadow-xl",
        "animate-in fade-in-0 zoom-in-95 duration-150",
        above && "-translate-y-full",
      )}
      style={{
        left,
        top: above ? top - GAP : top + GAP,
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

      {many && (
        <div className="flex items-center justify-between gap-2 border-t px-2 py-1.5 text-xs">
          <button
            type="button"
            aria-label="Previous memory"
            data-testid="bubble-prev"
            onClick={onPrev}
            disabled={atStart}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-30"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <span data-testid="bubble-position" className="text-muted-foreground">
            {index + 1} of {stack.length}
          </span>
          <button
            type="button"
            aria-label="Next memory"
            data-testid="bubble-next"
            onClick={onNext}
            disabled={atEnd}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-30"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>
      )}

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

      {/* Triangle tail (border-solid is required for the directional borders to
          render under Tailwind v4). */}
      <div
        className={cn(
          "absolute h-0 w-0 -translate-x-1/2 border-x-8 border-solid border-x-transparent",
          above ? "top-full border-t-[10px] border-t-popover" : "bottom-full border-b-[10px] border-b-popover",
        )}
        style={{ left: tailLeft }}
      />
    </div>,
    document.body,
  );
}
