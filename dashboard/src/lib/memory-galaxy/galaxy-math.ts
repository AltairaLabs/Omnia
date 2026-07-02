import type { GalaxyPoint } from "./types";
import type { Tier } from "@/lib/memory-analytics/types";
import { categoryColorHex, tierColorHex } from "@/lib/colors/category";

const TIERS: Tier[] = ["institutional", "agent", "user", "user_for_agent"];
const PADDING = 24;

export interface Transform { scale: number; offsetX: number; offsetY: number; }
export interface ScreenPos { x: number; y: number; }

export function fitTransform(points: GalaxyPoint[], width: number, height: number): Transform {
  if (points.length === 0) {
    return { scale: 1, offsetX: width / 2, offsetY: height / 2 };
  }
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  for (const p of points) {
    minX = Math.min(minX, p.x); maxX = Math.max(maxX, p.x);
    minY = Math.min(minY, p.y); maxY = Math.max(maxY, p.y);
  }
  const spanX = maxX - minX || 1;
  const spanY = maxY - minY || 1;
  const scale = Math.min((width - 2 * PADDING) / spanX, (height - 2 * PADDING) / spanY);
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  return { scale, offsetX: width / 2 - cx * scale, offsetY: height / 2 - cy * scale };
}

export function worldToScreen(p: GalaxyPoint, t: Transform): ScreenPos {
  return { x: p.x * t.scale + t.offsetX, y: p.y * t.scale + t.offsetY };
}

export function hitTest(points: GalaxyPoint[], pos: ScreenPos, t: Transform, radius: number): string | null {
  let best: string | null = null;
  let bestD = radius * radius;
  for (const p of points) {
    const s = worldToScreen(p, t);
    const dx = s.x - pos.x;
    const dy = s.y - pos.y;
    const d = dx * dx + dy * dy;
    if (d <= bestD) { bestD = d; best = p.id; }
  }
  return best;
}

export function colorForPoint(p: GalaxyPoint, colorBy: "tier" | "category"): string {
  if (colorBy === "tier") return tierColorHex(p.tier);
  return categoryColorHex(p.category);
}

export function isTierVisible(p: GalaxyPoint, hidden: Set<string>): boolean {
  return !hidden.has(p.tier);
}

export function matchesFilters(p: GalaxyPoint, filters: { category: string; search: string }): boolean {
  if (filters.category !== "all" && p.category !== filters.category) return false;
  if (filters.search) {
    const q = filters.search.toLowerCase();
    const hay = `${p.title ?? ""} ${p.preview ?? ""}`.toLowerCase();
    if (!hay.includes(q)) return false;
  }
  return true;
}

export function legendCounts(points: GalaxyPoint[]): Record<Tier, number> {
  const counts: Record<Tier, number> = { institutional: 0, agent: 0, user: 0, user_for_agent: 0 };
  for (const p of points) counts[p.tier]++;
  return counts;
}

// Which facet a point belongs to for the active color/filter dimension.
export function pointFacet(p: GalaxyPoint, dim: "tier" | "category"): string {
  return dim === "tier" ? p.tier : (p.category ?? "unknown");
}

// Count points per facet value for the active dimension.
export function facetCounts(points: GalaxyPoint[], dim: "tier" | "category"): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const p of points) {
    const k = pointFacet(p, dim);
    counts[k] = (counts[k] ?? 0) + 1;
  }
  return counts;
}

export function parseHiddenTiers(csv: string): Set<string> {
  return new Set(csv.split(",").map((s) => s.trim()).filter(Boolean));
}

export function serializeHiddenTiers(hidden: Set<string>): string {
  return TIERS.filter((t) => hidden.has(t)).join(",");
}
