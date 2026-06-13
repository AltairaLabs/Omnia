import { describe, it, expect } from "vitest";
import { nodeSize } from "./node-sizes";

describe("nodeSize", () => {
  it("sizes states/agents at the standard box", () => {
    expect(nodeSize("state")).toEqual({ width: 200, height: 68 });
    expect(nodeSize("agent")).toEqual({ width: 200, height: 68 });
  });
  it("sizes pseudo-states as small circles", () => {
    expect(nodeSize("initial")).toEqual({ width: 24, height: 24 });
    expect(nodeSize("final")).toEqual({ width: 24, height: 24 });
  });
  it("sizes variable lozenges and artifact parallelograms", () => {
    expect(nodeSize("variable")).toEqual({ width: 120, height: 30 });
    expect(nodeSize("artifact")).toEqual({ width: 150, height: 44 });
  });
  it("sizes arena harness nodes", () => {
    expect(nodeSize("scenario")).toEqual({ width: 170, height: 56 });
    expect(nodeSize("judge")).toEqual({ width: 170, height: 56 });
    expect(nodeSize("persona")).toEqual({ width: 170, height: 56 });
  });
});
