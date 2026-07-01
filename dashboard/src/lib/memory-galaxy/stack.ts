import type { GalaxyPoint } from "./types";

// Logic for the "stacked memories" popup: when a click lands on a dense cluster
// where many memories share (almost) the same coordinate, we open ONE popup and
// let the user page through them. These pure helpers own the browse order and
// index math so the canvas component (which is not unit-testable) stays thin.

// sortStack orders the overlapping points for browsing: grouped by category so
// paging moves category-by-category, with categories ranked by their nearest
// member (so the category you actually clicked into leads) and nearest-first
// within a category (so item 0 is the point closest to the click). `dist` maps a
// point to its squared distance from the click.
export function sortStack(
  points: GalaxyPoint[],
  dist: (p: GalaxyPoint) => number,
): GalaxyPoint[] {
  const catMin = new Map<string, number>();
  for (const p of points) {
    const c = p.category ?? "";
    const d = dist(p);
    const cur = catMin.get(c);
    if (cur === undefined || d < cur) catMin.set(c, d);
  }
  return [...points].sort((a, b) => {
    const ca = a.category ?? "";
    const cb = b.category ?? "";
    if (ca !== cb) return (catMin.get(ca) ?? 0) - (catMin.get(cb) ?? 0);
    return dist(a) - dist(b);
  });
}

// clampIndex constrains an index into [0, len-1], returning 0 for an empty stack.
export function clampIndex(index: number, len: number): number {
  if (len <= 0) return 0;
  return Math.max(0, Math.min(len - 1, index));
}

// removeAt drops the memory at `index` (e.g. after deleting it) and returns the
// shrunken stack plus the index to show next, clamped so it lands on the
// following memory (or the new last one).
export function removeAt(
  stack: GalaxyPoint[],
  index: number,
): { stack: GalaxyPoint[]; index: number } {
  const next = stack.filter((_, i) => i !== index);
  return { stack: next, index: clampIndex(index, next.length) };
}
