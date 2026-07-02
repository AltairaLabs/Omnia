/**
 * Color and label tokens for the memory-analytics page. Tier values are
 * also reused by the demo video and blog so they stay stable.
 */

import type { Tier } from "./types";

// Tier colors are token-driven via lib/colors/category (tierColorVar / tierColorHex).

export const TIER_LABELS: Record<Tier, string> = {
  institutional: "Institutional",
  agent: "Agent",
  user: "User",
  user_for_agent: "User-for-agent",
};

export const TIER_DESCRIPTIONS: Record<Tier, string> = {
  institutional:
    "Knowledge shared across every agent in the workspace — policies, product facts, brand voice.",
  agent:
    "Patterns this agent has learned from past conversations across all users.",
  user:
    "Things this agent remembers about a specific user across all of their agents — preferences, identity, history.",
  user_for_agent:
    "Things this agent remembers about a specific user in this agent only — per-agent personalisation.",
};
