/**
 * Color and label tokens for the memory-analytics page. Tier values are
 * also reused by the demo video and blog so they stay stable.
 */

import type { Tier } from "./types";

export const TIER_COLORS: Record<Tier, string> = {
  institutional: "hsl(217, 91%, 60%)",
  agent: "hsl(160, 84%, 39%)",
  user: "hsl(280, 75%, 60%)",
};

export const TIER_LABELS: Record<Tier, string> = {
  institutional: "Institutional",
  agent: "Agent",
  user: "User",
};

export const TIER_DESCRIPTIONS: Record<Tier, string> = {
  institutional:
    "Knowledge shared across every agent in the workspace — policies, product facts, brand voice.",
  agent:
    "Patterns this agent has learned from past conversations across all users.",
  user:
    "Things this agent remembers about a specific user — preferences, history, context.",
};

export const CATEGORY_COLORS: Record<string, string> = {
  "memory:context": "hsl(200, 80%, 55%)",
  "memory:identity": "hsl(15, 85%, 55%)",
  "memory:health": "hsl(0, 75%, 55%)",
  "memory:location": "hsl(45, 90%, 55%)",
  "memory:preferences": "hsl(140, 70%, 45%)",
  "memory:history": "hsl(260, 70%, 60%)",
  unknown: "hsl(0, 0%, 60%)",
};
