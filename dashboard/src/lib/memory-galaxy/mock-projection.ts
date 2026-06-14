import type { GalaxyPoint, GalaxyResponse } from "./types";
import type { Tier } from "@/lib/memory-analytics/types";

const TIERS: Tier[] = ["institutional", "agent", "user", "user_for_agent"];
const CATEGORIES = [
  "memory:identity", "memory:context", "memory:health",
  "memory:location", "memory:preferences", "memory:history",
];
const TOPICS = [
  "refunds and returns", "scheduling preferences", "billing questions",
  "product specs", "user identity", "support tone", "shipping policy", "account settings",
];

function rng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function clamp(n: number): number {
  return Math.max(-1, Math.min(1, n));
}

export function generateMockProjection({ seed, count }: { seed: number; count: number }): GalaxyResponse {
  const r = rng(seed);
  const clusters = TOPICS.map((topic) => ({
    topic,
    cx: r() * 1.6 - 0.8,
    cy: r() * 1.6 - 0.8,
    category: CATEGORIES[Math.floor(r() * CATEGORIES.length)],
  }));
  const points: GalaxyPoint[] = [];
  for (let i = 0; i < count; i++) {
    const c = clusters[Math.floor(r() * clusters.length)];
    const tier = TIERS[Math.floor(r() * TIERS.length)];
    const jitter = () => (r() + r() + r() - 1.5) * 0.18;
    points.push({
      id: `mock-${seed}-${i}`,
      x: clamp(c.cx + jitter()),
      y: clamp(c.cy + jitter()),
      tier,
      category: c.category,
      confidence: 0.4 + r() * 0.6,
      title: `${c.topic} #${i}`,
      preview: `A remembered detail about ${c.topic}.`,
      observedAt: "2026-06-10T00:00:00Z",
    });
  }
  return {
    model: "tsne", embeddingModel: "mock", embeddingDim: 0,
    total: count, capped: false, computedAt: "2026-06-14T00:00:00Z", points,
  };
}
