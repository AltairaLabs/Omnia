import { describe, it, expect } from "vitest";
import { PrometheusService } from "./prometheus-service";
import type { CostAllocationItem } from "./types";

function item(over: Partial<CostAllocationItem>): CostAllocationItem {
  return {
    agent: "a",
    namespace: "ns",
    provider: "openai",
    model: "gpt-4o",
    inputTokens: 0,
    outputTokens: 0,
    cacheHits: 0,
    requests: 0,
    inputCost: 0,
    outputCost: 0,
    cacheSavings: 0,
    totalCost: 0,
    ...over,
  };
}

// buildSummary is private; exercise it via a typed cast.
const buildSummary = (items: CostAllocationItem[]) =>
  (
    new PrometheusService() as unknown as {
      buildSummary(i: CostAllocationItem[]): {
        inputPercent: number;
        outputPercent: number;
      };
    }
  ).buildSummary(items);

describe("PrometheusService.buildSummary token percentages", () => {
  it("derives input/output percent from token counts, not cost", () => {
    // Output tokens are priced ~9x more here, so a cost-based split would be
    // ~10% / 90% — but these percentages label the "Total Tokens" card, so they
    // must reflect the token split (466 / 163), not the cost split.
    const summary = buildSummary([
      item({ inputTokens: 466, outputTokens: 163, inputCost: 1, outputCost: 9 }),
    ]);
    expect(Math.round(summary.inputPercent)).toBe(74); // 466 / 629
    expect(Math.round(summary.outputPercent)).toBe(26); // 163 / 629
  });

  it("returns 0 / 0 when there are no tokens", () => {
    const summary = buildSummary([]);
    expect(summary.inputPercent).toBe(0);
    expect(summary.outputPercent).toBe(0);
  });
});
