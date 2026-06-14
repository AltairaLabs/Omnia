"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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

interface MemoryGalaxyProps {
  points: GalaxyPoint[];
  colorBy: "tier" | "category";
  hiddenTiers: Set<string>;
  filters: { category: string; search: string };
  onSelect: (point: GalaxyPoint) => void;
}

function drawPoints(
  ctx: CanvasRenderingContext2D,
  points: GalaxyPoint[],
  colorBy: "tier" | "category",
  hiddenTiers: Set<string>,
  filters: { category: string; search: string },
  t: Transform,
): void {
  for (const p of points) {
    if (!isTierVisible(p, hiddenTiers)) continue;
    const dim = !matchesFilters(p, filters);
    const s = worldToScreen(p, t);
    ctx.globalAlpha = dim ? 0.12 : 0.9;
    ctx.fillStyle = colorForPoint(p, colorBy);
    ctx.beginPath();
    ctx.arc(s.x, s.y, 3 + p.confidence * 3, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
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

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const { width, height } = canvas;
    ctx.clearRect(0, 0, width, height);
    const t: Transform = fitTransform(points, width, height);
    drawPoints(ctx, points, colorBy, hiddenTiers, filters, t);
  }, [points, colorBy, hiddenTiers, filters]);

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

  const visiblePoints = points.filter((p) => isTierVisible(p, hiddenTiers));

  const pickAt = (e: React.MouseEvent<HTMLCanvasElement>): GalaxyPoint | null => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const pos = { x: e.clientX - rect.left, y: e.clientY - rect.top };
    const t = fitTransform(points, canvas.width, canvas.height);
    const id = hitTest(visiblePoints, pos, t, 8);
    return id ? (visiblePoints.find((p) => p.id === id) ?? null) : null;
  };

  return (
    <div className="relative h-[600px] w-full rounded-lg border bg-[#0b1020]">
      <canvas
        ref={canvasRef}
        data-testid="memory-galaxy-canvas"
        className="h-full w-full cursor-pointer"
        onMouseMove={(e) => setHovered(pickAt(e))}
        onMouseLeave={() => setHovered(null)}
        onClick={(e) => {
          const p = pickAt(e);
          if (p) onSelect(p);
        }}
      />
      {hovered && (
        <div className="pointer-events-none absolute left-3 top-3 max-w-xs rounded-md bg-background/95 p-2 text-xs shadow">
          <div className="font-medium">{hovered.title}</div>
          <div className="text-muted-foreground">{hovered.preview}</div>
        </div>
      )}
    </div>
  );
}
