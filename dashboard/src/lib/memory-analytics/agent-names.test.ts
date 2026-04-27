import { describe, it, expect } from "vitest";
import type { AgentRuntime } from "@/types";
import { agentNameByUidMap, resolveAgentRows } from "./agent-names";

function makeAgent(name: string, uid: string | undefined): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name, uid },
    spec: {},
  } as AgentRuntime;
}

describe("agentNameByUidMap", () => {
  it("indexes agents by their UID", () => {
    const map = agentNameByUidMap([
      makeAgent("support", "uid-1"),
      makeAgent("billing", "uid-2"),
    ]);
    expect(map.get("uid-1")).toBe("support");
    expect(map.get("uid-2")).toBe("billing");
  });

  it("skips agents without a UID", () => {
    const map = agentNameByUidMap([makeAgent("pending", undefined)]);
    expect(map.size).toBe(0);
  });
});

describe("resolveAgentRows", () => {
  const nameByUid = new Map([
    ["uid-1", "support"],
    ["uid-2", "billing"],
  ]);

  it("replaces UID keys with agent names", () => {
    const out = resolveAgentRows(
      [{ key: "uid-1", value: 5, count: 5 }],
      nameByUid,
    );
    expect(out).toEqual([{ key: "support", value: 5, count: 5 }]);
  });

  it("falls back to the UID when no agent matches", () => {
    const out = resolveAgentRows(
      [{ key: "uid-orphan", value: 3, count: 3 }],
      nameByUid,
    );
    expect(out).toEqual([{ key: "uid-orphan", value: 3, count: 3 }]);
  });

  it("preserves value and count fields", () => {
    const out = resolveAgentRows(
      [{ key: "uid-1", value: 50, count: 42 }],
      nameByUid,
    );
    expect(out[0].value).toBe(50);
    expect(out[0].count).toBe(42);
  });

  it("returns an empty array for empty input", () => {
    expect(resolveAgentRows([], nameByUid)).toEqual([]);
  });
});
