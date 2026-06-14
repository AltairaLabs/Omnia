"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTheme } from "next-themes";
import { Plus, Minus, Maximize, Tag, Gauge, Clock } from "lucide-react";
import { usePersistedViewMode } from "@/hooks/use-persisted-view-mode";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import {
  fitTransform,
  worldToScreen,
  colorForPoint,
  matchesFilters,
  pointFacet,
  type Transform,
} from "@/lib/memory-galaxy/galaxy-math";
import { TIER_LABELS } from "@/lib/memory-analytics/colors";
import type { Tier } from "@/lib/memory-analytics/types";
import { cn } from "@/lib/utils";

type Dimension = "tier" | "category";

interface View {
  zoom: number;
  panX: number;
  panY: number;
}
interface ScreenPos {
  x: number;
  y: number;
}
interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}
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
// Confidence → radius: non-linear so a bell-curved distribution still spreads
// visibly (the busy mid/high band gets most of the size range).
const MIN_RADIUS = 2;
const CONF_SPREAD = 9;
const CONF_GAMMA = 1.8;
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
  onSelect: (point: GalaxyPoint) => void;
}

function project(p: GalaxyPoint, fit: Transform, view: View): ScreenPos {
  const b = worldToScreen(p, fit);
  return { x: b.x * view.zoom + view.panX, y: b.y * view.zoom + view.panY };
}

// Logarithmic zoom growth, on top of a strong non-linear confidence term.
function sizeFactor(zoom: number): number {
  return Math.max(0.7, 1 + Math.log2(zoom) * 1.2);
}

function pointRadius(p: GalaxyPoint, sf: number): number {
  return (MIN_RADIUS + Math.pow(p.confidence, CONF_GAMMA) * CONF_SPREAD) * sf;
}

function overlaps(a: Box, b: Box): boolean {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

function onScreen(s: ScreenPos, w: number, h: number): boolean {
  return s.x > -60 && s.x < w + 60 && s.y > -20 && s.y < h + 20;
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

function drawPoolBall(ctx: CanvasRenderingContext2D, s: ScreenPos, r: number, confidence: number): void {
  const inner = r * 0.62;
  ctx.beginPath();
  ctx.arc(s.x, s.y, inner, 0, Math.PI * 2);
  ctx.fillStyle = "rgba(255,255,255,0.92)";
  ctx.fill();
  ctx.fillStyle = "#0b1020";
  ctx.font = `${Math.round(inner * 1.1)}px sans-serif`;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(String(Math.round(confidence * 100)), s.x, s.y + 0.5);
}

interface DrawOpts {
  showConfidence: boolean;
  ageFade: boolean;
  now: number;
}

// 0 = fresh, 1 = at/past expiry (created + TTL). 0 when there is no TTL.
function lifeFraction(p: GalaxyPoint, now: number): number {
  if (!p.observedAt || !p.expiresAt) return 0;
  const created = Date.parse(p.observedAt);
  const expires = Date.parse(p.expiresAt);
  if (expires <= created) return 0;
  return Math.max(0, Math.min(1, (now - created) / (expires - created)));
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
  onSelect,
}: Readonly<MemoryGalaxyProps>) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [hovered, setHovered] = useState<GalaxyPoint | null>(null);
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
    if (labelsOn && view.zoom >= LABEL_MIN_ZOOM) {
      drawLabels(ctx, labelCandidates(points, colorBy, hidden, filters, scene), colorBy, width, isDark);
    }
  }, [points, colorBy, hidden, filters, view, labelsOn, showConfidence, ageFade, isDark]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const resize = () => {
      const rect = canvas.getBoundingClientRect();
      canvas.width = rect.width;
      canvas.height = rect.height;
      draw();
    };
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(canvas);
    return () => ro.disconnect();
  }, [draw]);

  useEffect(() => {
    draw();
  }, [draw]);

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

  const onMouseUp = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const d = drag.current;
    if (d.active && !d.moved) {
      const p = pickAt(e.clientX, e.clientY);
      if (p) onSelect(p);
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
