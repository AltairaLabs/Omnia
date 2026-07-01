"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTheme } from "next-themes";
import { Plus, Minus, Maximize, Tag, Gauge, Clock, X } from "lucide-react";
import { usePersistedViewMode } from "@/hooks/use-persisted-view-mode";
import { MemoryGalaxyBubble } from "./memory-galaxy-bubble";
import { sortStack, clampIndex, removeAt } from "@/lib/memory-galaxy/stack";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import {
  fitTransform,
  colorForPoint,
  matchesFilters,
  pointFacet,
  type Transform,
} from "@/lib/memory-galaxy/galaxy-math";
import {
  project,
  sizeFactor,
  pointRadius,
  overlaps,
  onScreen,
  computeBubbles,
  lifeFraction,
  type View,
  type ScreenPos,
  type Box,
} from "@/lib/memory-galaxy/galaxy-layout";
import { drawPoolBall, drawRings, drawSearchRings } from "@/lib/memory-galaxy/galaxy-draw";
import { matchFitView } from "@/lib/memory-galaxy/galaxy-search";
import { TIER_LABELS } from "@/lib/memory-analytics/colors";
import type { Tier } from "@/lib/memory-analytics/types";
import { cn } from "@/lib/utils";

type Dimension = "tier" | "category";

interface Scene {
  fit: Transform;
  view: View;
  w: number;
  h: number;
}

const DEFAULT_VIEW: View = { zoom: 1, panX: 0, panY: 0 };
const MIN_ZOOM = 0.5;
const MAX_ZOOM = 16;
const DRAG_THRESHOLD = 3;
const LABEL_MIN_ZOOM = 2;
const MAX_LABELS = 160;
const POOL_BALL_MIN_RADIUS = 9;
const HIT_TOLERANCE = 4;
// Explicit white-on-dark control styling (theme variants render dark-on-dark
// against the galaxy background, so don't rely on them here).
const TOGGLE_BTN = "flex h-8 w-8 items-center justify-center rounded-md transition-colors";
const TOGGLE_ON = "bg-white/20 text-white hover:bg-white/25";
const TOGGLE_OFF = "text-white/45 hover:bg-white/10 hover:text-white/80";
const TOGGLE_PLAIN = "text-white/85 hover:bg-white/15 hover:text-white";

interface MemoryGalaxyProps {
  points: GalaxyPoint[];
  colorBy: Dimension;
  hidden: Set<string>;
  filters: { search: string };
  onDelete: (id: string) => void;
}

function categoryLabel(cat?: string): string {
  if (!cat) return "Uncategorized";
  const s = cat.replace(/^memory:/, "");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

// The label heading is the dimension NOT used for color.
function headingFor(p: GalaxyPoint, colorBy: Dimension): string {
  return colorBy === "tier" ? categoryLabel(p.category) : TIER_LABELS[p.tier as Tier];
}

function isVisible(p: GalaxyPoint, hidden: Set<string>, colorBy: Dimension): boolean {
  return !hidden.has(pointFacet(p, colorBy));
}

interface DrawOpts {
  showConfidence: boolean;
  ageFade: boolean;
  now: number;
}

function drawPoints(
  ctx: CanvasRenderingContext2D,
  points: GalaxyPoint[],
  colorBy: Dimension,
  hidden: Set<string>,
  filters: { search: string },
  scene: Scene,
  opts: DrawOpts,
): void {
  const sf = sizeFactor(scene.view.zoom);
  for (const p of points) {
    if (!isVisible(p, hidden, colorBy)) continue;
    const s = project(p, scene.fit, scene.view);
    if (!onScreen(s, scene.w, scene.h)) continue;
    const dim = !matchesFilters(p, { category: "all", search: filters.search });
    const r = pointRadius(p, sf);
    const age = opts.ageFade ? Math.max(0.12, 1 - lifeFraction(p, opts.now) * 0.85) : 1;
    ctx.globalAlpha = (dim ? 0.1 : 0.9) * age;
    ctx.fillStyle = colorForPoint(p, colorBy);
    ctx.beginPath();
    ctx.arc(s.x, s.y, r, 0, Math.PI * 2);
    ctx.fill();
    if (opts.showConfidence && !dim && r >= POOL_BALL_MIN_RADIUS) drawPoolBall(ctx, s, r, p.confidence);
  }
  ctx.globalAlpha = 1;
}

function labelCandidates(
  points: GalaxyPoint[],
  colorBy: Dimension,
  hidden: Set<string>,
  filters: { search: string },
  scene: Scene,
): Array<{ p: GalaxyPoint; s: ScreenPos }> {
  const out: Array<{ p: GalaxyPoint; s: ScreenPos }> = [];
  for (const p of points) {
    if (!isVisible(p, hidden, colorBy)) continue;
    if (!matchesFilters(p, { category: "all", search: filters.search })) continue;
    const s = project(p, scene.fit, scene.view);
    if (onScreen(s, scene.w, scene.h)) out.push({ p, s });
  }
  out.sort((a, b) => b.p.confidence - a.p.confidence);
  return out;
}

// Heading = the non-color dimension; type underneath.
function drawLabels(
  ctx: CanvasRenderingContext2D,
  candidates: Array<{ p: GalaxyPoint; s: ScreenPos }>,
  colorBy: Dimension,
  width: number,
  isDark: boolean,
): void {
  const labelBg = isDark ? "rgba(11,16,32,0.72)" : "rgba(248,250,252,0.82)";
  const labelText = isDark ? "rgba(226,232,240,0.95)" : "rgba(15,23,42,0.95)";
  const subText = isDark ? "rgba(148,163,184,0.9)" : "rgba(71,85,105,0.9)";
  const placed: Box[] = [];
  ctx.textAlign = "left";
  ctx.textBaseline = "middle";
  for (const { p, s } of candidates) {
    if (placed.length >= MAX_LABELS) break;
    ctx.font = "11px sans-serif";
    const heading = headingFor(p, colorBy);
    const box: Box = { x: s.x + 10, y: s.y - 8, w: ctx.measureText(heading).width + 8, h: 26 };
    if (box.x + box.w > width || placed.some((q) => overlaps(box, q))) continue;
    placed.push(box);
    ctx.fillStyle = labelBg;
    ctx.fillRect(box.x - 3, box.y, box.w, box.h);
    ctx.fillStyle = labelText;
    ctx.fillText(heading, box.x, s.y - 1);
    ctx.font = "9px sans-serif";
    ctx.fillStyle = subText;
    ctx.fillText(p.type ?? "—", box.x, s.y + 11);
  }
}

export function MemoryGalaxy({
  points,
  colorBy,
  hidden,
  filters,
  onDelete,
}: Readonly<MemoryGalaxyProps>) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [hovered, setHovered] = useState<GalaxyPoint | null>(null);
  // The memories stacked at the currently-open location, ordered for browsing,
  // and which one the carousel is showing. A cluster of overlapping points opens
  // ONE popup that pages through them instead of a window per point.
  const [openStack, setOpenStack] = useState<GalaxyPoint[]>([]);
  const [stackIndex, setStackIndex] = useState(0);
  // The bubble anchors to the first (lead) memory of the stack, so its position
  // stays put while you page. computeBubbles/drawRings still work off an id set.
  const openLeadId = openStack[0]?.id ?? null;
  const openIds = useMemo(
    () => new Set(openLeadId ? [openLeadId] : []),
    [openLeadId],
  );
  const [size, setSize] = useState({ w: 0, h: 0, left: 0, top: 0 });
  const [view, setView] = useState<View>(DEFAULT_VIEW);
  const [labelsPref, setLabelsPref] = usePersistedViewMode<"on" | "off">(
    "omnia-memory-galaxy-labels",
    "on",
  );
  const [confPref, setConfPref] = usePersistedViewMode<"on" | "off">(
    "omnia-memory-galaxy-confidence",
    "on",
  );
  const [agePref, setAgePref] = usePersistedViewMode<"on" | "off">(
    "omnia-memory-galaxy-agefade",
    "on",
  );
  const labelsOn = labelsPref === "on";
  const showConfidence = confPref === "on";
  const ageFade = agePref === "on";
  const { resolvedTheme } = useTheme();
  const isDark = resolvedTheme !== "light";
  const drag = useRef({ active: false, moved: false, startX: 0, startY: 0, panX: 0, panY: 0 });
  const prevZoom = useRef(view.zoom);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const searchZoomTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Auto-close open bubbles when zooming out, debounced so small adjustments
  // don't dismiss them.
  useEffect(() => {
    const zoomedOut = view.zoom < prevZoom.current - 0.001;
    prevZoom.current = view.zoom;
    if (!zoomedOut || openStack.length === 0) return;
    if (closeTimer.current) clearTimeout(closeTimer.current);
    closeTimer.current = setTimeout(() => {
      setOpenStack([]);
      setStackIndex(0);
    }, 450);
    return () => {
      if (closeTimer.current) clearTimeout(closeTimer.current);
    };
  }, [view.zoom, openStack.length]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const { width, height } = canvas;
    ctx.clearRect(0, 0, width, height);
    const fit = fitTransform(points, width, height);
    const scene: Scene = { fit, view, w: width, h: height };
    drawPoints(ctx, points, colorBy, hidden, filters, scene, { showConfidence, ageFade, now: Date.now() });
    drawRings(ctx, points, openIds, fit, view, isDark);
    drawSearchRings(ctx, points, filters.search, fit, view);
    if (labelsOn && view.zoom >= LABEL_MIN_ZOOM) {
      drawLabels(ctx, labelCandidates(points, colorBy, hidden, filters, scene), colorBy, width, isDark);
    }
  }, [points, colorBy, hidden, filters, view, labelsOn, showConfidence, ageFade, isDark, openIds]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const resize = () => {
      const rect = canvas.getBoundingClientRect();
      canvas.width = rect.width;
      canvas.height = rect.height;
      setSize({ w: rect.width, h: rect.height, left: rect.left, top: rect.top });
      draw();
    };
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(canvas);
    window.addEventListener("scroll", resize, true);
    window.addEventListener("resize", resize);
    return () => {
      ro.disconnect();
      window.removeEventListener("scroll", resize, true);
      window.removeEventListener("resize", resize);
    };
  }, [draw]);

  useEffect(() => {
    draw();
  }, [draw]);

  // Debounced zoom-to-matches: framing the matching points makes an active
  // search obviously "do something"; clearing it returns to the full view.
  useEffect(() => {
    if (size.w === 0) return;
    if (searchZoomTimer.current) clearTimeout(searchZoomTimer.current);
    searchZoomTimer.current = setTimeout(() => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      if (!filters.search) {
        setView(DEFAULT_VIEW);
        return;
      }
      const fit = fitTransform(points, canvas.width, canvas.height);
      const v = matchFitView(points, filters.search, fit, canvas.width, canvas.height, MIN_ZOOM, MAX_ZOOM);
      if (v) setView(v);
    }, 400);
    return () => {
      if (searchZoomTimer.current) clearTimeout(searchZoomTimer.current);
    };
  }, [filters.search, points, size.w]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const rect = canvas.getBoundingClientRect();
      const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
      setView((v) => {
        const zoom = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, v.zoom * factor));
        const k = zoom / v.zoom;
        const fx = e.clientX - rect.left;
        const fy = e.clientY - rect.top;
        return { zoom, panX: fx - (fx - v.panX) * k, panY: fy - (fy - v.panY) * k };
      });
    };
    canvas.addEventListener("wheel", onWheel, { passive: false });
    return () => canvas.removeEventListener("wheel", onWheel);
  }, []);

  const visiblePoints = points.filter((p) => isVisible(p, hidden, colorBy));

  const pickAt = (clientX: number, clientY: number): GalaxyPoint | null => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const mx = clientX - rect.left;
    const my = clientY - rect.top;
    const fit = fitTransform(points, canvas.width, canvas.height);
    const sf = sizeFactor(view.zoom);
    let best: GalaxyPoint | null = null;
    let bestD = Infinity;
    for (const p of visiblePoints) {
      const s = project(p, fit, view);
      const r = pointRadius(p, sf) + HIT_TOLERANCE;
      const dx = s.x - mx;
      const dy = s.y - my;
      const d = dx * dx + dy * dy;
      if (d <= r * r && d < bestD) {
        bestD = d;
        best = p;
      }
    }
    return best;
  };

  const onMouseDown = (e: React.MouseEvent<HTMLCanvasElement>) => {
    drag.current = {
      active: true,
      moved: false,
      startX: e.clientX,
      startY: e.clientY,
      panX: view.panX,
      panY: view.panY,
    };
  };

  const onMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const d = drag.current;
    if (d.active) {
      const dx = e.clientX - d.startX;
      const dy = e.clientY - d.startY;
      if (Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD) d.moved = true;
      setView((v) => ({ ...v, panX: d.panX + dx, panY: d.panY + dy }));
      return;
    }
    setHovered(pickAt(e.clientX, e.clientY));
  };

  // pickAllAt returns every visible point under the click (a dense cluster can
  // stack many at one spot), ordered for browsing by sortStack.
  const pickAllAt = (clientX: number, clientY: number): GalaxyPoint[] => {
    const canvas = canvasRef.current;
    if (!canvas) return [];
    const rect = canvas.getBoundingClientRect();
    const mx = clientX - rect.left;
    const my = clientY - rect.top;
    const fit = fitTransform(points, canvas.width, canvas.height);
    const sf = sizeFactor(view.zoom);
    const distById = new Map<string, number>();
    const hits: GalaxyPoint[] = [];
    for (const p of visiblePoints) {
      const s = project(p, fit, view);
      const r = pointRadius(p, sf) + HIT_TOLERANCE;
      const dx = s.x - mx;
      const dy = s.y - my;
      const d = dx * dx + dy * dy;
      if (d <= r * r) {
        hits.push(p);
        distById.set(p.id, d);
      }
    }
    return sortStack(hits, (p) => distById.get(p.id) ?? 0);
  };

  const closeStack = () => {
    setOpenStack([]);
    setStackIndex(0);
  };

  const openStackAt = (stack: GalaxyPoint[]) => {
    // Clicking the already-open cluster (same lead memory) closes it; a new
    // cluster replaces it and resets to the first memory.
    if (stack.length === 0 || stack[0].id === openLeadId) {
      closeStack();
      return;
    }
    setOpenStack(stack);
    setStackIndex(0);
  };

  const showPrev = () => setStackIndex((i) => clampIndex(i - 1, openStack.length));
  const showNext = () => setStackIndex((i) => clampIndex(i + 1, openStack.length));

  const deleteCurrent = (id: string) => {
    onDelete(id);
    const next = removeAt(openStack, stackIndex);
    setOpenStack(next.stack);
    setStackIndex(next.index);
  };

  const onMouseUp = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const d = drag.current;
    if (d.active && !d.moved) {
      openStackAt(pickAllAt(e.clientX, e.clientY));
    }
    d.active = false;
  };

  const zoomByButton = (factor: number) => {
    const canvas = canvasRef.current;
    const cx = canvas ? canvas.width / 2 : 0;
    const cy = canvas ? canvas.height / 2 : 0;
    setView((v) => {
      const zoom = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, v.zoom * factor));
      const k = zoom / v.zoom;
      return { zoom, panX: cx - (cx - v.panX) * k, panY: cy - (cy - v.panY) * k };
    });
  };

  const openBubbles = computeBubbles(
    points,
    openIds,
    view,
    size,
    openIds.size > 0 ? { left: size.left, top: size.top } : null,
  );

  return (
    <div className="relative h-[70vh] min-h-[360px] w-full overflow-hidden rounded-lg border bg-slate-50 dark:bg-[#0b1020]">
      <canvas
        ref={canvasRef}
        data-testid="memory-galaxy-canvas"
        className="h-full w-full cursor-grab active:cursor-grabbing"
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={onMouseUp}
        onMouseLeave={() => {
          setHovered(null);
          drag.current.active = false;
        }}
      />

      <div className="absolute bottom-3 right-3 z-10 flex flex-col gap-0.5 rounded-lg bg-slate-900/90 p-1 shadow-lg ring-1 ring-white/15 backdrop-blur">
        {openIds.size > 0 && (
          <>
            <button
              type="button"
              aria-label="Close all popups"
              title="Close all popups"
              data-testid="close-all"
              onClick={closeStack}
              className={cn(TOGGLE_BTN, TOGGLE_PLAIN)}
            >
              <X className="h-4 w-4" />
            </button>
            <div className="my-0.5 h-px bg-white/15" />
          </>
        )}
        <button
          type="button"
          aria-label={labelsOn ? "Hide labels" : "Show labels"}
          aria-pressed={labelsOn}
          title="Toggle labels"
          data-testid="toggle-labels"
          onClick={() => setLabelsPref(labelsOn ? "off" : "on")}
          className={cn(TOGGLE_BTN, labelsOn ? TOGGLE_ON : TOGGLE_OFF)}
        >
          <Tag className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label={showConfidence ? "Hide confidence" : "Show confidence"}
          aria-pressed={showConfidence}
          title="Toggle confidence scores"
          data-testid="toggle-confidence"
          onClick={() => setConfPref(showConfidence ? "off" : "on")}
          className={cn(TOGGLE_BTN, showConfidence ? TOGGLE_ON : TOGGLE_OFF)}
        >
          <Gauge className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label={ageFade ? "Disable age fade" : "Enable age fade"}
          aria-pressed={ageFade}
          title="Toggle age fade"
          data-testid="toggle-age-fade"
          onClick={() => setAgePref(ageFade ? "off" : "on")}
          className={cn(TOGGLE_BTN, ageFade ? TOGGLE_ON : TOGGLE_OFF)}
        >
          <Clock className="h-4 w-4" />
        </button>
        <div className="my-0.5 h-px bg-white/15" />
        <button type="button" aria-label="Zoom in" title="Zoom in" onClick={() => zoomByButton(1.3)} className={cn(TOGGLE_BTN, TOGGLE_PLAIN)}>
          <Plus className="h-4 w-4" />
        </button>
        <button type="button" aria-label="Zoom out" title="Zoom out" onClick={() => zoomByButton(1 / 1.3)} className={cn(TOGGLE_BTN, TOGGLE_PLAIN)}>
          <Minus className="h-4 w-4" />
        </button>
        <button type="button" aria-label="Reset view" title="Reset view" onClick={() => setView(DEFAULT_VIEW)} className={cn(TOGGLE_BTN, TOGGLE_PLAIN)}>
          <Maximize className="h-4 w-4" />
        </button>
      </div>

      {openBubbles[0] && openStack.length > 0 && (
        <MemoryGalaxyBubble
          stack={openStack}
          index={stackIndex}
          left={openBubbles[0].left}
          top={openBubbles[0].top}
          placement={openBubbles[0].placement}
          tailOffset={openBubbles[0].tailOffset}
          onPrev={showPrev}
          onNext={showNext}
          onClose={closeStack}
          onDelete={deleteCurrent}
        />
      )}

      {hovered && (
        <div className="pointer-events-none absolute left-3 top-3 max-w-xs rounded-md bg-background/95 p-2 text-xs shadow">
          <div className="font-medium">{hovered.title}</div>
          <div className="text-muted-foreground">
            {categoryLabel(hovered.category)}
            {hovered.type ? ` · ${hovered.type}` : ""}
          </div>
        </div>
      )}
    </div>
  );
}
