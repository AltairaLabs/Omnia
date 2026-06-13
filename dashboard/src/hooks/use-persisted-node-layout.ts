"use client";

import { useCallback } from "react";
import type { Node } from "@xyflow/react";

// Per-graph node positions persisted to localStorage (a personal view preference,
// not server state). Keyed by the caller, e.g. "promptpack:<ns>:<name>",
// "agent:<ns>:<name>", "topology:<ns>". Mirrors the existing notes-storage
// (localStorage) pattern rather than writing UI state onto CRDs.

type SavedPositions = Record<string, { x: number; y: number }>;

function read(key: string): SavedPositions {
  if (!key || typeof window === "undefined") return {};
  try {
    return JSON.parse(window.localStorage.getItem(key) ?? "{}") as SavedPositions;
  } catch {
    return {};
  }
}

function write(key: string, saved: SavedPositions): void {
  if (!key) return;
  try {
    window.localStorage.setItem(key, JSON.stringify(saved));
  } catch {
    /* quota exceeded / storage disabled — ignore */
  }
}

export interface PersistedNodeLayout {
  /** Override each node's position with a saved one, if present. */
  applyLayout: <T extends Node>(nodes: T[]) => T[];
  /** Persist a node's position after a drag. */
  onNodeDragStop: (event: unknown, node: Node) => void;
  /** Clear the saved layout for this key. */
  reset: () => void;
}

export function usePersistedNodeLayout(storageKey: string): PersistedNodeLayout {
  const applyLayout = useCallback(
    <T extends Node>(nodes: T[]): T[] => {
      const saved = read(storageKey);
      return nodes.map((n) => {
        const p = saved[n.id];
        return p ? { ...n, position: p } : n;
      });
    },
    [storageKey],
  );

  const onNodeDragStop = useCallback(
    (_event: unknown, node: Node) => {
      const saved = read(storageKey);
      saved[node.id] = { x: node.position.x, y: node.position.y };
      write(storageKey, saved);
    },
    [storageKey],
  );

  const reset = useCallback(() => {
    try {
      window.localStorage.removeItem(storageKey);
    } catch {
      /* ignore */
    }
  }, [storageKey]);

  return { applyLayout, onNodeDragStop, reset };
}
