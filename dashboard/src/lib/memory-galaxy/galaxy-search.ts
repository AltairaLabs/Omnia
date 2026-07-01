import type { GalaxyPoint } from "./types";
import { worldToScreen, type Transform } from "./galaxy-math";
import type { View } from "./galaxy-layout";

// Search logic for the Memory Galaxy: which points match a free-text query, how
// many, and where the view should move to frame them. Pure + unit-tested; the
// canvas component only wires these to state.

// matchesSearch tests a point's title + preview against a lowercased query.
// An empty query matches everything. Consent-masked points (null title/preview)
// can never match — their searchable text is stripped before it reaches here.
export function matchesSearch(p: GalaxyPoint, query: string): boolean {
  if (!query) return true;
  const q = query.toLowerCase();
  return `${p.title ?? ""} ${p.preview ?? ""}`.toLowerCase().includes(q);
}

// countMatches counts points matching a non-empty query (0 for an empty query).
export function countMatches(points: GalaxyPoint[], query: string): number {
  if (!query) return 0;
  let n = 0;
  for (const p of points) if (matchesSearch(p, query)) n++;
  return n;
}

const FIT_PADDING = 60;

// matchFitView returns a View (zoom + pan) that centres and frames every point
// matching the query inside a w×h canvas, clamped to [minZoom, maxZoom]. Returns
// null when the query is empty, the canvas has no size, or nothing matches — the
// caller then leaves the view untouched.
export function matchFitView(
  points: GalaxyPoint[],
  query: string,
  fit: Transform,
  w: number,
  h: number,
  minZoom: number,
  maxZoom: number,
): View | null {
  if (!query || w === 0 || h === 0) return null;
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  let count = 0;
  for (const p of points) {
    if (!matchesSearch(p, query)) continue;
    const s = worldToScreen(p, fit);
    if (s.x < minX) minX = s.x;
    if (s.x > maxX) maxX = s.x;
    if (s.y < minY) minY = s.y;
    if (s.y > maxY) maxY = s.y;
    count++;
  }
  if (count === 0) return null;
  const boxW = Math.max(1, maxX - minX);
  const boxH = Math.max(1, maxY - minY);
  const zoom = Math.max(
    minZoom,
    Math.min(maxZoom, Math.min((w - 2 * FIT_PADDING) / boxW, (h - 2 * FIT_PADDING) / boxH)),
  );
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  return { zoom, panX: w / 2 - cx * zoom, panY: h / 2 - cy * zoom };
}
