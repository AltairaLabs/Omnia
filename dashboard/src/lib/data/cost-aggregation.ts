/**
 * Pure assembler that builds the dashboard CostData shape from session-api
 * provider-calls aggregate rows. Transport-agnostic: the caller fetches the
 * rows (from one or more service-group sources, concatenated) and passes them
 * here. Totals come straight from exact cost_usd / token sums; per-model
 * pricing is used only to split input vs output cost.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { getModelPricing } from "../pricing";
import type {
  CostData,
  CostSummary,
  CostAllocationItem,
  ProviderCost,
  ModelCost,
  CostTimeSeriesPoint,
} from "./types";
import type { ProviderCallAggregateRow } from "./provider-calls-service";

const KEY_DELIM = "|";

/** Aggregate rows for one workspace, already merged across service-group sources. */
export interface CostAggregateInput {
  /** keyed "provider|model|agent" */
  cost: ProviderCallAggregateRow[];
  inputTokens: ProviderCallAggregateRow[];
  outputTokens: ProviderCallAggregateRow[];
  cachedTokens: ProviderCallAggregateRow[];
  requests: ProviderCallAggregateRow[];
  /** keyed "timestamp|provider" */
  costByHourProvider: ProviderCallAggregateRow[];
  namespace: string;
}

const MODEL_DISPLAY_NAMES: Record<string, string> = {
  "claude-3-opus": "Claude 3 Opus",
  "claude-3-sonnet": "Claude 3 Sonnet",
  "claude-3-haiku": "Claude 3 Haiku",
  "claude-sonnet-4": "Claude Sonnet 4",
  "claude-opus-4": "Claude Opus 4",
  "gpt-4": "GPT-4",
  "gpt-4-turbo": "GPT-4 Turbo",
  "gpt-4o": "GPT-4o",
  "gpt-4o-mini": "GPT-4o Mini",
  "gpt-3.5-turbo": "GPT-3.5 Turbo",
};

function modelDisplayName(model: string): string {
  return MODEL_DISPLAY_NAMES[model] ?? model;
}

// Provider display names mirror the previous Prometheus path.
function providerDisplayName(p: string): string {
  if (p === "anthropic") return "Anthropic";
  if (p === "openai") return "OpenAI";
  return p;
}

function emptySummary(): CostSummary {
  return {
    totalCost: 0,
    totalInputCost: 0,
    totalOutputCost: 0,
    totalCacheSavings: 0,
    totalRequests: 0,
    totalTokens: 0,
    inputTokens: 0,
    outputTokens: 0,
    projectedMonthlyCost: 0,
    inputPercent: 0,
    outputPercent: 0,
  };
}

/** Accumulate one metric's rows into the per-(provider,model,agent) map. */
function accumulate(
  map: Map<string, CostAllocationItem>,
  namespace: string,
  rows: ProviderCallAggregateRow[],
  field: keyof Pick<
    CostAllocationItem,
    "inputTokens" | "outputTokens" | "cacheHits" | "requests" | "totalCost"
  >,
): void {
  for (const row of rows) {
    const [provider = "unknown", model = "unknown", agent = "unknown"] =
      row.key.split(KEY_DELIM);
    const key = `${provider}${KEY_DELIM}${model}${KEY_DELIM}${agent}`;
    let item = map.get(key);
    if (!item) {
      item = {
        agent,
        namespace,
        provider,
        model,
        inputTokens: 0,
        outputTokens: 0,
        cacheHits: 0,
        requests: 0,
        inputCost: 0,
        outputCost: 0,
        cacheSavings: 0,
        totalCost: 0,
      };
      map.set(key, item);
    }
    item[field] += row.value;
  }
}

function applyPricing(item: CostAllocationItem): void {
  const pricing = getModelPricing(item.model);
  if (!pricing) return;
  item.inputCost = (item.inputTokens / 1_000_000) * pricing.inputPer1M;
  item.outputCost = (item.outputTokens / 1_000_000) * pricing.outputPer1M;
  if (pricing.cachePer1M != null) {
    item.cacheSavings =
      (item.cacheHits / 1_000_000) * (pricing.inputPer1M - pricing.cachePer1M);
  }
}

function buildByAgent(input: CostAggregateInput): CostAllocationItem[] {
  const map = new Map<string, CostAllocationItem>();
  accumulate(map, input.namespace, input.cost, "totalCost");
  accumulate(map, input.namespace, input.inputTokens, "inputTokens");
  accumulate(map, input.namespace, input.outputTokens, "outputTokens");
  accumulate(map, input.namespace, input.cachedTokens, "cacheHits");
  accumulate(map, input.namespace, input.requests, "requests");
  const items = Array.from(map.values());
  items.forEach(applyPricing);
  return items.sort((a, b) => b.totalCost - a.totalCost);
}

function buildSummary(byAgent: CostAllocationItem[]): CostSummary {
  const s = emptySummary();
  for (const i of byAgent) {
    s.totalCost += i.totalCost;
    s.totalInputCost += i.inputCost;
    s.totalOutputCost += i.outputCost;
    s.totalCacheSavings += i.cacheSavings;
    s.totalRequests += i.requests;
    s.inputTokens += i.inputTokens;
    s.outputTokens += i.outputTokens;
  }
  s.totalTokens = s.inputTokens + s.outputTokens;
  s.projectedMonthlyCost = s.totalCost * 30;
  if (s.totalTokens > 0) {
    s.inputPercent = (s.inputTokens / s.totalTokens) * 100;
    s.outputPercent = (s.outputTokens / s.totalTokens) * 100;
  }
  return s;
}

function normalizeProvider(p: string): string {
  return p === "claude" ? "anthropic" : p;
}

function buildByProvider(byAgent: CostAllocationItem[]): ProviderCost[] {
  const map = new Map<string, ProviderCost>();
  for (const i of byAgent) {
    const provider = normalizeProvider(i.provider);
    let p = map.get(provider);
    if (!p) {
      p = { name: providerDisplayName(provider), provider, cost: 0, requests: 0, tokens: 0 };
      map.set(provider, p);
    }
    p.cost += i.totalCost;
    p.requests += i.requests;
    p.tokens += i.inputTokens + i.outputTokens;
  }
  return Array.from(map.values()).sort((a, b) => b.cost - a.cost);
}

function buildByModel(byAgent: CostAllocationItem[]): ModelCost[] {
  const map = new Map<string, ModelCost>();
  for (const i of byAgent) {
    let m = map.get(i.model);
    if (!m) {
      m = {
        model: i.model,
        displayName: modelDisplayName(i.model),
        provider: i.provider,
        cost: 0,
        requests: 0,
        tokens: 0,
      };
      map.set(i.model, m);
    }
    m.cost += i.totalCost;
    m.requests += i.requests;
    m.tokens += i.inputTokens + i.outputTokens;
  }
  return Array.from(map.values()).sort((a, b) => b.cost - a.cost);
}

function buildTimeSeries(rows: ProviderCallAggregateRow[]): CostTimeSeriesPoint[] {
  const byTs = new Map<string, Record<string, number>>();
  for (const row of rows) {
    const [timestamp = "", providerRaw = "unknown"] = row.key.split(KEY_DELIM);
    if (!timestamp) continue;
    const provider = normalizeProvider(providerRaw);
    let point = byTs.get(timestamp);
    if (!point) {
      point = {};
      byTs.set(timestamp, point);
    }
    point[provider] = (point[provider] ?? 0) + row.value;
  }
  return Array.from(byTs.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([timestamp, byProvider]) => ({
      timestamp,
      byProvider,
      total: Object.values(byProvider).reduce((sum, v) => sum + v, 0),
    }));
}

/** Build the dashboard CostData from merged provider-calls aggregate rows. */
export function buildCostData(input: CostAggregateInput): CostData {
  const byAgent = buildByAgent(input);
  return {
    available: true,
    summary: buildSummary(byAgent),
    byAgent,
    byProvider: buildByProvider(byAgent),
    byModel: buildByModel(byAgent),
    timeSeries: buildTimeSeries(input.costByHourProvider),
  };
}
