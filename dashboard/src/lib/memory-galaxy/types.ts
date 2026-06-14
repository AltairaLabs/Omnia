import type { Tier } from "@/lib/memory-analytics/types";

export interface GalaxyPoint {
  id: string;
  x: number; // normalized ~[-1, 1]
  y: number; // normalized ~[-1, 1]
  tier: Tier;
  category?: string; // e.g. "memory:identity"
  confidence: number; // 0..1
  title?: string;
  preview?: string;
  observedAt?: string;
}

export interface GalaxyResponse {
  model: "tsne" | "pca";
  // Which representation the coordinates were derived from: dense embeddings
  // (clusters by meaning) or TF-IDF/BM25 term vectors from content (clusters by
  // shared words) for FTS-only deployments. Drives the "semantic vs lexical"
  // hint in the UI. Optional for forward-compat.
  projectionInput?: "embedding" | "tfidf";
  embeddingModel: string;
  embeddingDim: number;
  total: number;
  capped: boolean;
  computedAt: string;
  points: GalaxyPoint[];
}
