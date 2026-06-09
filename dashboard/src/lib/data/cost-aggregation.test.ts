/**
 * Tests for the pure cost assembler.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect } from "vitest";
import { buildCostData, type CostAggregateInput } from "./cost-aggregation";

const input: CostAggregateInput = {
  // matrix keyed "provider|model|agent"
  cost: [{ key: "openai|gpt-4|chatbot", value: 0.03, count: 2 }],
  inputTokens: [{ key: "openai|gpt-4|chatbot", value: 150, count: 2 }],
  outputTokens: [{ key: "openai|gpt-4|chatbot", value: 15, count: 2 }],
  cachedTokens: [{ key: "openai|gpt-4|chatbot", value: 0, count: 2 }],
  requests: [{ key: "openai|gpt-4|chatbot", value: 2, count: 2 }],
  // series keyed "timestamp|provider"
  costByHourProvider: [{ key: "2026-06-09T13:00:00Z|openai", value: 0.03, count: 2 }],
  namespace: "default",
};

describe("buildCostData", () => {
  it("uses exact cost_usd for totals (no extrapolation)", () => {
    const data = buildCostData(input);
    expect(data.available).toBe(true);
    expect(data.summary.totalCost).toBeCloseTo(0.03, 9);
    expect(data.summary.totalTokens).toBe(165);
    expect(data.summary.inputTokens).toBe(150);
    expect(data.summary.totalRequests).toBe(2);
  });

  it("produces byAgent / byProvider / byModel rows", () => {
    const data = buildCostData(input);
    expect(data.byAgent).toHaveLength(1);
    expect(data.byAgent[0]).toMatchObject({ agent: "chatbot", model: "gpt-4", provider: "openai" });
    expect(data.byProvider[0]).toMatchObject({ provider: "openai", cost: 0.03 });
    expect(data.byModel[0]).toMatchObject({ model: "gpt-4" });
  });

  it("builds a per-provider time series", () => {
    const data = buildCostData(input);
    expect(data.timeSeries).toHaveLength(1);
    expect(data.timeSeries[0].byProvider.openai).toBeCloseTo(0.03, 9);
    expect(data.timeSeries[0].total).toBeCloseTo(0.03, 9);
  });

  it("sums duplicate keys across sources", () => {
    const merged: CostAggregateInput = {
      ...input,
      cost: [
        { key: "openai|gpt-4|chatbot", value: 0.03, count: 2 },
        { key: "openai|gpt-4|chatbot", value: 0.01, count: 1 },
      ],
    };
    expect(buildCostData(merged).summary.totalCost).toBeCloseTo(0.04, 9);
  });

  it("derives input/output cost split from per-model pricing", () => {
    const data = buildCostData(input);
    // gpt-4 has nonzero pricing in lib/pricing; split must be > 0.
    expect(data.summary.totalInputCost).toBeGreaterThan(0);
    expect(data.summary.totalOutputCost).toBeGreaterThan(0);
  });

  it("token percentages come from token counts, not cost", () => {
    const data = buildCostData(input);
    expect(data.summary.inputPercent).toBeCloseTo((150 / 165) * 100, 6);
    expect(data.summary.outputPercent).toBeCloseTo((15 / 165) * 100, 6);
  });

  it("normalizes claude provider to anthropic in breakdowns", () => {
    const claudeInput: CostAggregateInput = {
      cost: [{ key: "claude|claude-3-5-sonnet|support", value: 0.05, count: 1 }],
      inputTokens: [{ key: "claude|claude-3-5-sonnet|support", value: 300, count: 1 }],
      outputTokens: [{ key: "claude|claude-3-5-sonnet|support", value: 500, count: 1 }],
      cachedTokens: [],
      requests: [{ key: "claude|claude-3-5-sonnet|support", value: 1, count: 1 }],
      costByHourProvider: [{ key: "2026-06-09T13:00:00Z|claude", value: 0.05, count: 1 }],
      namespace: "default",
    };
    const data = buildCostData(claudeInput);
    expect(data.byProvider[0].provider).toBe("anthropic");
    expect(data.timeSeries[0].byProvider.anthropic).toBeCloseTo(0.05, 9);
  });

  it("handles unknown provider/model (no pricing) and keeps the raw provider name", () => {
    const unknown: CostAggregateInput = {
      cost: [{ key: "google|gemini-2|bot", value: 0.07, count: 1 }],
      inputTokens: [{ key: "google|gemini-2|bot", value: 100, count: 1 }],
      outputTokens: [{ key: "google|gemini-2|bot", value: 20, count: 1 }],
      cachedTokens: [],
      requests: [{ key: "google|gemini-2|bot", value: 1, count: 1 }],
      costByHourProvider: [],
      namespace: "default",
    };
    const data = buildCostData(unknown);
    // Unknown provider keeps its raw name; total cost is still exact.
    expect(data.byProvider[0].provider).toBe("google");
    expect(data.byProvider[0].name).toBe("google");
    expect(data.summary.totalCost).toBeCloseTo(0.07, 9);
    // No pricing for an unknown model -> input/output cost split stays 0.
    expect(data.summary.totalInputCost).toBe(0);
    expect(data.byModel[0].displayName).toBe("gemini-2");
  });

  it("skips empty-timestamp rows and sorts the series chronologically", () => {
    const data = buildCostData({
      ...input,
      costByHourProvider: [
        { key: "|openai", value: 0.02, count: 1 }, // skipped (no timestamp)
        { key: "2026-06-09T14:00:00Z|openai", value: 0.01, count: 1 }, // later, listed first
        { key: "2026-06-09T13:00:00Z|openai", value: 0.03, count: 2 }, // earlier
      ],
    });
    expect(data.timeSeries).toHaveLength(2);
    expect(data.timeSeries[0].timestamp).toBe("2026-06-09T13:00:00Z");
    expect(data.timeSeries[1].timestamp).toBe("2026-06-09T14:00:00Z");
  });

  it("returns empty breakdowns for empty input", () => {
    const empty: CostAggregateInput = {
      cost: [], inputTokens: [], outputTokens: [], cachedTokens: [], requests: [],
      costByHourProvider: [], namespace: "default",
    };
    const data = buildCostData(empty);
    expect(data.summary.totalCost).toBe(0);
    expect(data.byAgent).toEqual([]);
    expect(data.timeSeries).toEqual([]);
  });
});
