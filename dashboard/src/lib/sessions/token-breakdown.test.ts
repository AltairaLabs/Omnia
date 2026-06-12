import { describe, it, expect } from "vitest";
import { providerCallsBySource } from "./token-breakdown";
import type { ProviderCall } from "@/types";

function call(partial: Partial<ProviderCall>): ProviderCall {
  return {
    id: Math.random().toString(36).slice(2),
    sessionId: "s1",
    provider: "openai",
    model: "gpt-4o",
    status: "completed",
    createdAt: "2026-06-12T00:00:00Z",
    ...partial,
  };
}

describe("providerCallsBySource", () => {
  it("groups completed calls by source and sums tokens + cost", () => {
    const rows = providerCallsBySource([
      call({ source: "agent", inputTokens: 100, outputTokens: 10, costUsd: 0.01 }),
      call({ source: "agent", inputTokens: 50, outputTokens: 5, costUsd: 0.005 }),
      call({ source: "selfplay", provider: "ollama", model: "llama3.2:3b", inputTokens: 300, outputTokens: 30, costUsd: 0 }),
    ]);

    expect(rows).toHaveLength(2);
    const agent = rows.find((r) => r.source === "agent")!;
    expect(agent.label).toBe("Agent");
    expect(agent.inputTokens).toBe(150);
    expect(agent.outputTokens).toBe(15);
    expect(agent.costUsd).toBeCloseTo(0.015);
    expect(agent.count).toBe(2);

    const sp = rows.find((r) => r.source === "selfplay")!;
    expect(sp.label).toBe("Self-play");
    expect(sp.inputTokens).toBe(300);
  });

  it("treats empty/missing source as agent", () => {
    const rows = providerCallsBySource([
      call({ source: undefined, inputTokens: 10 }),
      call({ source: "", inputTokens: 20 }),
    ]);
    expect(rows).toHaveLength(1);
    expect(rows[0].source).toBe("agent");
    expect(rows[0].inputTokens).toBe(30);
  });

  it("ignores non-completed calls", () => {
    const rows = providerCallsBySource([
      call({ source: "selfplay", status: "failed", inputTokens: 999 }),
      call({ source: "selfplay", status: "completed", inputTokens: 5 }),
    ]);
    expect(rows).toHaveLength(1);
    expect(rows[0].inputTokens).toBe(5);
  });

  it("orders agent, self-play, judge first then others", () => {
    const rows = providerCallsBySource([
      call({ source: "zebra" }),
      call({ source: "judge" }),
      call({ source: "selfplay" }),
      call({ source: "agent" }),
    ]);
    expect(rows.map((r) => r.source)).toEqual(["agent", "selfplay", "judge", "zebra"]);
  });
});
