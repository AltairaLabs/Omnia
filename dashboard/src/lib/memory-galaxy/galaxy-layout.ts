import type { GalaxyPoint } from "./types";
import { fitTransform, worldToScreen, type Transform } from "./galaxy-math";

// Pure geometry + layout helpers for the Memory Galaxy canvas. Kept out of the
// (canvas-bound, coverage-excluded) component so they can be reasoned about and
// tested on their own.

export interface View {
  zoom: number;
  panX: number;
  panY: number;
}

export interface ScreenPos {
  x: number;
  y: number;
}

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

// Confidence → radius: non-linear so a bell-curved distribution still spreads
// visibly (the busy mid/high band gets most of the size range).
export const MIN_RADIUS = 2;
export const CONF_SPREAD = 9;
export const CONF_GAMMA = 1.8;

export function project(p: GalaxyPoint, fit: Transform, view: View): ScreenPos {
  const b = worldToScreen(p, fit);
  return { x: b.x * view.zoom + view.panX, y: b.y * view.zoom + view.panY };
}

// Logarithmic zoom growth, on top of a strong non-linear confidence term.
export function sizeFactor(zoom: number): number {
  return Math.max(0.7, 1 + Math.log2(zoom) * 1.2);
}

export function pointRadius(p: GalaxyPoint, sf: number): number {
  return (MIN_RADIUS + Math.pow(p.confidence, CONF_GAMMA) * CONF_SPREAD) * sf;
}

export function overlaps(a: Box, b: Box): boolean {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

export function onScreen(s: ScreenPos, w: number, h: number): boolean {
  return s.x > -60 && s.x < w + 60 && s.y > -20 && s.y < h + 20;
}

// 0 = fresh, 1 = at/past expiry (created + TTL). 0 when there is no TTL.
export function lifeFraction(p: GalaxyPoint, now: number): number {
  if (!p.observedAt || !p.expiresAt) return 0;
  const created = Date.parse(p.observedAt);
  const expires = Date.parse(p.expiresAt);
  if (expires <= created) return 0;
  return Math.max(0, Math.min(1, (now - created) / (expires - created)));
}

export interface BubblePos {
  p: GalaxyPoint;
  left: number; // viewport x of the node
  top: number; // viewport y of the node
  placement: "above" | "below";
  tailOffset: number;
}

// Live viewport anchors for the open bubbles, recomputed from the view each
// render so they follow their points on pan/zoom. Only points currently on the
// canvas get a bubble.
export function computeBubbles(
  points: GalaxyPoint[],
  openIds: Set<string>,
  view: View,
  size: { w: number; h: number },
  rect: { left: number; top: number } | null,
): BubblePos[] {
  const out: BubblePos[] = [];
  if (openIds.size === 0 || size.w === 0 || !rect) return out;
  const fit = fitTransform(points, size.w, size.h);
  const vw = window.innerWidth;
  for (const p of points) {
    if (!openIds.has(p.id)) continue;
    const s = project(p, fit, view);
    if (s.x < 0 || s.x > size.w || s.y < 0 || s.y > size.h) continue;
    const nodeX = rect.left + s.x;
    const cardLeft = Math.max(150, Math.min(vw - 150, nodeX));
    out.push({
      p,
      left: cardLeft,
      top: rect.top + s.y,
      placement: s.y > size.h / 2 ? "above" : "below",
      tailOffset: Math.max(-120, Math.min(120, nodeX - cardLeft)),
    });
  }
  return out;
}
