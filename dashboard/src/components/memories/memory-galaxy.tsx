"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Plus, Minus, Maximize } from "lucide-react";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import {
  fitTransform,
  worldToScreen,
  hitTest,
  colorForPoint,
  isTierVisible,
  matchesFilters,
  type Transform,
} from "@/lib/memory-galaxy/galaxy-math";
import { TIER_LABELS } from "@/lib/memory-analytics/colors";
import { Button } from "@/components/ui/button";

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

const DEFAULT_VIEW: View = { zoom: 1, panX: 0, panY: 0 };
const MIN_ZOOM = 0.5;
const MAX_ZOOM = 16;
const DRAG_THRESHOLD = 3;
const LABEL_MIN_ZOOM = 2; // start showing titles
const DETAIL_ZOOM = 5; // also show a metadata line
const MAX_LABELS = 160;

interface MemoryGalaxyProps {
  points: GalaxyPoint[];
  colorBy: "tier" | "category";
  hiddenTiers: Set<string>;
  filters: { category: string; search: string };
  onSelect: (point: GalaxyPoint) => void;
}

function project(p: GalaxyPoint, fit: Transform, view: View): ScreenPos {
  const b = worldToScreen(p, fit);
  return { x: b.x * view.zoom + view.panX, y: b.y * view.zoom + view.panY };
}

// Logarithmic, so points grow gently with zoom instead of ballooning linearly.
function sizeFactor(zoom: number): number {
  return Math.max(0.7, 1 + Math.log2(zoom) * 0.35);
}

function overlaps(a: Box, b: Box): boolean {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

function onScreen(s: ScreenPos, w: number, h: number): boolean {
  return s.x > -60 && s.x < w + 60 && s.y > -20 && s.y < h + 20;
}

function drawPoints(
  ctx: CanvasRenderingContext2D,
  points: GalaxyPoint[],
  colorBy: "tier" | "category",
  hiddenTiers: Set<string>,
  filters: { category: string; search: string },
  fit: Transform,
  view: View,
): void {
  const sf = sizeFactor(view.zoom);
  for (const p of points) {
    if (!isTierVisible(p, hiddenTiers)) continue;
    const dim = !matchesFilters(p, filters);
    const s = project(p, fit, view);
    ctx.globalAlpha = dim ? 0.1 : 0.9;
    ctx.fillStyle = colorForPoint(p, colorBy);
    ctx.beginPath();
    ctx.arc(s.x, s.y, (2.5 + p.confidence * 3) * sf, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
}

// Visible, on-screen, non-dimmed points with a title, highest-confidence first
// (so important labels win collisions).
function labelCandidates(
  points: GalaxyPoint[],
  hiddenTiers: Set<string>,
  filters: { category: string; search: string },
  fit: Transform,
  view: View,
  w: number,
  h: number,
): Array<{ p: GalaxyPoint; s: ScreenPos }> {
  const out: Array<{ p: GalaxyPoint; s: ScreenPos }> = [];
  for (const p of points) {
    if (!p.title || !isTierVisible(p, hiddenTiers) || !matchesFilters(p, filters)) continue;
    const s = project(p, fit, view);
    if (onScreen(s, w, h)) out.push({ p, s });
  }
  out.sort((a, b) => b.p.confidence - a.p.confidence);
  return out;
}

// Semantic zoom: progressively label points once zoomed in, skipping any label
// that would collide with one already placed.
function drawLabels(
  ctx: CanvasRenderingContext2D,
  candidates: Array<{ p: GalaxyPoint; s: ScreenPos }>,
  detail: boolean,
  width: number,
): void {
  const placed: Box[] = [];
  ctx.textBaseline = "middle";
  for (const { p, s } of candidates) {
    if (placed.length >= MAX_LABELS) break;
    ctx.font = "11px sans-serif";
    const title = p.title ?? "";
    const box: Box = { x: s.x + 8, y: s.y - 7, w: ctx.measureText(title).width + 6, h: detail ? 24 : 14 };
    if (box.x + box.w > width || placed.some((q) => overlaps(box, q))) continue;
    placed.push(box);
    ctx.fillStyle = "rgba(11,16,32,0.72)";
    ctx.fillRect(box.x - 3, box.y, box.w, box.h);
    ctx.fillStyle = "rgba(226,232,240,0.95)";
    ctx.fillText(title, box.x, s.y);
    if (detail) {
      ctx.font = "9px sans-serif";
      ctx.fillStyle = "rgba(148,163,184,0.9)";
      ctx.fillText(`${TIER_LABELS[p.tier]} · ${Math.round(p.confidence * 100)}%`, box.x, s.y + 11);
    }
  }
}

export function MemoryGalaxy({
  points,
  colorBy,
  hiddenTiers,
  filters,
  onSelect,
}: Readonly<MemoryGalaxyProps>) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [hovered, setHovered] = useState<GalaxyPoint | null>(null);
  const [view, setView] = useState<View>(DEFAULT_VIEW);
  const drag = useRef({ active: false, moved: false, startX: 0, startY: 0, panX: 0, panY: 0 });

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const { width, height } = canvas;
    ctx.clearRect(0, 0, width, height);
    const fit = fitTransform(points, width, height);
    drawPoints(ctx, points, colorBy, hiddenTiers, filters, fit, view);
    if (view.zoom >= LABEL_MIN_ZOOM) {
      const candidates = labelCandidates(points, hiddenTiers, filters, fit, view, width, height);
      drawLabels(ctx, candidates, view.zoom >= DETAIL_ZOOM, width);
    }
  }, [points, colorBy, hiddenTiers, filters, view]);

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

  const visiblePoints = points.filter((p) => isTierVisible(p, hiddenTiers));

  const pickAt = (clientX: number, clientY: number): GalaxyPoint | null => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const fit = fitTransform(points, canvas.width, canvas.height);
    const base = {
      x: (clientX - rect.left - view.panX) / view.zoom,
      y: (clientY - rect.top - view.panY) / view.zoom,
    };
    const id = hitTest(visiblePoints, base, fit, 8 / view.zoom);
    return id ? (visiblePoints.find((p) => p.id === id) ?? null) : null;
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
    <div className="relative h-[600px] w-full overflow-hidden rounded-lg border bg-[#0b1020]">
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

      <div className="absolute bottom-3 right-3 flex flex-col gap-1">
        <Button size="icon" variant="secondary" aria-label="Zoom in" onClick={() => zoomByButton(1.3)}>
          <Plus className="h-4 w-4" />
        </Button>
        <Button size="icon" variant="secondary" aria-label="Zoom out" onClick={() => zoomByButton(1 / 1.3)}>
          <Minus className="h-4 w-4" />
        </Button>
        <Button size="icon" variant="secondary" aria-label="Reset view" onClick={() => setView(DEFAULT_VIEW)}>
          <Maximize className="h-4 w-4" />
        </Button>
      </div>

      {hovered && (
        <div className="pointer-events-none absolute left-3 top-3 max-w-xs rounded-md bg-background/95 p-2 text-xs shadow">
          <div className="font-medium">{hovered.title}</div>
          <div className="text-muted-foreground">{hovered.preview}</div>
        </div>
      )}
    </div>
  );
}
