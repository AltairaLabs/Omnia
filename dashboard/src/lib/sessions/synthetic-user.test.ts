import { describe, it, expect } from "vitest";
import { syntheticUserInfo, syntheticUserLabel } from "./synthetic-user";

describe("syntheticUserInfo", () => {
  it("returns null for a real-user session (no source:arena tag)", () => {
    expect(syntheticUserInfo(undefined)).toBeNull();
    expect(syntheticUserInfo([])).toBeNull();
    expect(syntheticUserInfo(["source:interactive", "user:abc"])).toBeNull();
  });

  it("identifies self-play when a persona tag is present", () => {
    const info = syntheticUserInfo([
      "source:arena",
      "arena-job:rag-hero-loadtest",
      "persona:sre-user",
    ]);
    expect(info).toEqual({ kind: "self-play", persona: "sre-user" });
  });

  it("identifies scenario when arena-driven but no persona", () => {
    expect(syntheticUserInfo(["source:arena", "scenario:incident-runbook"])).toEqual({
      kind: "scenario",
    });
  });
});

describe("syntheticUserLabel", () => {
  it("includes the persona for self-play", () => {
    expect(syntheticUserLabel({ kind: "self-play", persona: "sre-user" })).toBe(
      "Self-play · sre-user"
    );
  });

  it("falls back to a bare label when no persona", () => {
    expect(syntheticUserLabel({ kind: "self-play" })).toBe("Self-play");
    expect(syntheticUserLabel({ kind: "scenario" })).toBe("Scenario");
  });
});
