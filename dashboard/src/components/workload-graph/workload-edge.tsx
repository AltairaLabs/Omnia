"use client";

import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath, type EdgeProps } from "@xyflow/react";
import type { Point } from "./layout";

function unit(a: Point, b: Point): Point {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const len = Math.hypot(dx, dy) || 1;
  return { x: dx / len, y: dy / len };
}

// Build an SVG path through elk's waypoints with rounded corners.
export function roundedPath(points: Point[], radius = 10): string {
  if (points.length < 2) return "";
  let d = `M ${points[0].x},${points[0].y}`;
  for (let i = 1; i < points.length - 1; i++) {
    const prev = points[i - 1];
    const corner = points[i];
    const next = points[i + 1];
    const r1 = Math.min(radius, Math.hypot(corner.x - prev.x, corner.y - prev.y) / 2);
    const r2 = Math.min(radius, Math.hypot(next.x - corner.x, next.y - corner.y) / 2);
    const dIn = unit(prev, corner);
    const dOut = unit(corner, next);
    const enter = { x: corner.x - dIn.x * r1, y: corner.y - dIn.y * r1 };
    const exit = { x: corner.x + dOut.x * r2, y: corner.y + dOut.y * r2 };
    d += ` L ${enter.x},${enter.y} Q ${corner.x},${corner.y} ${exit.x},${exit.y}`;
  }
  const last = points[points.length - 1];
  d += ` L ${last.x},${last.y}`;
  return d;
}

export function midpoint(points: Point[]): Point {
  if (points.length === 0) return { x: 0, y: 0 };
  return points[Math.floor(points.length / 2)];
}

export function WorkloadEdge(props: Readonly<EdgeProps>) {
  const { id, data, style, markerEnd, label, sourceX, sourceY, targetX, targetY } = props;
  const points = (data as { points?: Point[] } | undefined)?.points;

  let path: string;
  let lx: number;
  let ly: number;
  if (points && points.length >= 2) {
    path = roundedPath(points);
    const m = midpoint(points);
    lx = m.x;
    ly = m.y;
  } else {
    const [p, labelX, labelY] = getSmoothStepPath({ sourceX, sourceY, targetX, targetY });
    path = p;
    lx = labelX;
    ly = labelY;
  }

  return (
    <>
      <BaseEdge id={id} path={path} style={style} markerEnd={markerEnd} />
      {label && (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan absolute rounded border bg-background/85 px-1 text-[11px] leading-tight text-muted-foreground"
            style={{ transform: `translate(-50%,-50%) translate(${lx}px,${ly}px)` }}
          >
            {label}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

export const workloadEdgeTypes = { workloadEdge: WorkloadEdge };
