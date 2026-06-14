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
import { Button } from "@/components/ui/button";

interface View {
  zoom: number;
  panX: number;
  panY: number;
}

const DEFAULT_VIEW: View = { zoom: 1, panX: 0, panY: 0 };
const MIN_ZOOM = 0.5;
const MAX_ZOOM = 16;
const DRAG_THRESHOLD = 3;

interface MemoryGalaxyProps {
  points: GalaxyPoint[];
  colorBy: "tier" | "category";
  hiddenTiers: Set<string>;
  filters: { category: string; search: string };
  onSelect: (point: GalaxyPoint) => void;
}

// screen position after the fit transform AND the interactive view (zoom/pan).
function project(p: GalaxyPoint, fit: Transform, view: View): { x: number; y: number } {
  const b = worldToScreen(p, fit);
  return { x: b.x * view.zoom + view.panX, y: b.y * view.zoom + view.panY };
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
  for (const p of points) {
    if (!isTierVisible(p, hiddenTiers)) continue;
    const dim = !matchesFilters(p, filters);
    const s = project(p, fit, view);
    ctx.globalAlpha = dim ? 0.1 : 0.9;
    ctx.fillStyle = colorForPoint(p, colorBy);
    ctx.beginPath();
    ctx.arc(s.x, s.y, 2.5 + p.confidence * 3, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
}

// clamp a new zoom and adjust pan so the (fx, fy) focal point stays put.
function zoomAround(view: View, factor: number, fx: number, fy: number): View {
  const zoom = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, view.zoom * factor));
  const k = zoom / view.zoom;
  return { zoom, panX: fx - (fx - view.panX) * k, panY: fy - (fy - view.panY) * k };
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
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    const fit = fitTransform(points, canvas.width, canvas.height);
    drawPoints(ctx, points, colorBy, hiddenTiers, filters, fit, view);
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

  // Native wheel listener (passive:false) so we can preventDefault and
  // zoom toward the cursor.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const rect = canvas.getBoundingClientRect();
      const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
      setView((v) => zoomAround(v, factor, e.clientX - rect.left, e.clientY - rect.top));
    };
    canvas.addEventListener("wheel", onWheel, { passive: false });
    return () => canvas.removeEventListener("wheel", onWheel);
  }, []);

  const visiblePoints = points.filter((p) => isTierVisible(p, hiddenTiers));

  const pickAt = (clientX: number, clientY: number): GalaxyPoint | null => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const mx = clientX - rect.left;
    const my = clientY - rect.top;
    const fit = fitTransform(points, canvas.width, canvas.height);
    // invert the view: base screen = (screen - pan) / zoom
    const base = { x: (mx - view.panX) / view.zoom, y: (my - view.panY) / view.zoom };
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
    setView((v) => zoomAround(v, factor, cx, cy));
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
