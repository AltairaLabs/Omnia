import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CostBreakdownTable } from "./cost-breakdown-table";
import type { CostAllocationItem } from "@/lib/data/types";

// A cache-heavy gpt-4o agent (issue #1489 numbers): the cached-input cost is the
// dominant line, so the visible cost columns must include it and reconcile to Total.
const ragHero: CostAllocationItem = {
  agent: "rag-hero",
  namespace: "demo",
  provider: "openai",
  model: "gpt-4o",
  inputTokens: 19365,
  outputTokens: 6558,
  cacheHits: 155264,
  requests: 523,
  inputCost: 0.0484125,
  outputCost: 0.06558,
  cachedCost: 0.19408,
  cacheSavings: 0.19408,
  totalCost: 0.30807,
};

describe("CostBreakdownTable", () => {
  it("renders a Cached Cost column", () => {
    render(<CostBreakdownTable data={[ragHero]} />);
    expect(screen.getByText("Cached Cost")).toBeInTheDocument();
    expect(screen.getByText("Cache Savings")).toBeInTheDocument();
  });

  it("surfaces the cached-input cost and populates cache savings", () => {
    render(<CostBreakdownTable data={[ragHero]} />);
    // Cached cost ($0.194) shown in the agent row and the totals row.
    expect(screen.getAllByText("$0.194").length).toBeGreaterThanOrEqual(1);
    // Cache savings is no longer blank for an OpenAI cache-heavy agent.
    expect(screen.getAllByText("-$0.194").length).toBeGreaterThanOrEqual(1);
  });

  it("includes cached tokens in the Tokens count", () => {
    render(<CostBreakdownTable data={[ragHero]} />);
    // 19365 + 6558 + 155264 = 181187 -> "181.2K" (cached folded in, not 25.9K).
    expect(screen.getAllByText("181.2K").length).toBeGreaterThanOrEqual(1);
  });

  it("renders an empty state with no data", () => {
    render(<CostBreakdownTable data={[]} />);
    expect(
      screen.getByText("No cost breakdown data available"),
    ).toBeInTheDocument();
  });
});
