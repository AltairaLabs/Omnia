import type { GalaxyPoint } from "./types";
import type { Transform } from "./galaxy-math";
import {
  project,
  sizeFactor,
  pointRadius,
  onScreen,
  type ScreenPos,
  type View,
} from "./galaxy-layout";
import { matchesSearch } from "./galaxy-search";

// Canvas draw helpers for the Memory Galaxy. Split out of the (already
// coverage-excluded) component so it stays under the file-length cap; these are
// pure ctx side effects, verified visually / via E2E rather than unit tests.

// The confidence number rendered inside a large enough point.
export function drawPoolBall(
  ctx: CanvasRenderingContext2D,
  s: ScreenPos,
  r: number,
  confidence: number,
): void {
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

// Highlight ring around points that currently have an open bubble, so it's
// obvious which node a bubble belongs to.
export function drawRings(
  ctx: CanvasRenderingContext2D,
  points: GalaxyPoint[],
  openIds: Set<string>,
  fit: Transform,
  view: View,
  isDark: boolean,
): void {
  if (openIds.size === 0) return;
  const sf = sizeFactor(view.zoom);
  ctx.lineWidth = 2;
  ctx.strokeStyle = isDark ? "rgba(255,255,255,0.95)" : "rgba(15,23,42,0.95)";
  for (const p of points) {
    if (!openIds.has(p.id)) continue;
    const s = project(p, fit, view);
    ctx.beginPath();
    ctx.arc(s.x, s.y, pointRadius(p, sf) + 4, 0, Math.PI * 2);
    ctx.stroke();
  }
}

// Amber ring around every point matching the active search query, so matches
// pop against the dimmed non-matches. No-op when the query is empty.
export function drawSearchRings(
  ctx: CanvasRenderingContext2D,
  points: GalaxyPoint[],
  query: string,
  fit: Transform,
  view: View,
): void {
  if (!query) return;
  const sf = sizeFactor(view.zoom);
  ctx.lineWidth = 2;
  ctx.strokeStyle = "rgba(250,204,21,0.95)"; // amber-400
  for (const p of points) {
    if (!matchesSearch(p, query)) continue;
    const s = project(p, fit, view);
    if (!onScreen(s, ctx.canvas.width, ctx.canvas.height)) continue;
    ctx.beginPath();
    ctx.arc(s.x, s.y, pointRadius(p, sf) + 3, 0, Math.PI * 2);
    ctx.stroke();
  }
}
