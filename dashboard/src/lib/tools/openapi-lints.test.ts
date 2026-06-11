import { describe, it, expect } from "vitest";
import { computeToolLints } from "./openapi-lints";

describe("computeToolLints", () => {
  it("warns when the description is empty", () => {
    const lints = computeToolLints({ name: "getPet", description: "", inputSchema: {} });
    expect(lints.some((l) => l.id === "weak-description")).toBe(true);
  });
  it("warns when the description is just METHOD /path", () => {
    const lints = computeToolLints({ name: "getPet", description: "GET /pets/{id}", inputSchema: {} });
    expect(lints.some((l) => l.id === "weak-description")).toBe(true);
  });
  it("does not warn on a real summary", () => {
    const lints = computeToolLints({ name: "getPet", description: "Fetch a pet by id", inputSchema: {} });
    expect(lints.some((l) => l.id === "weak-description")).toBe(false);
  });
  it("warns about required fields with no description", () => {
    const lints = computeToolLints({
      name: "getPet", description: "Fetch a pet",
      inputSchema: { type: "object", properties: { id: { type: "string" }, q: { type: "string", description: "search" } }, required: ["id"] },
    });
    const lint = lints.find((l) => l.id === "undescribed-required");
    expect(lint).toBeDefined();
    expect(lint!.message).toContain("id");
  });
  it("does not warn when required fields are described", () => {
    const lints = computeToolLints({
      name: "getPet", description: "Fetch a pet",
      inputSchema: { type: "object", properties: { id: { type: "string", description: "the pet id" } }, required: ["id"] },
    });
    expect(lints.some((l) => l.id === "undescribed-required")).toBe(false);
  });
  it("returns no lints for a well-formed tool", () => {
    expect(computeToolLints({ name: "getPet", description: "Fetch a pet by id", inputSchema: { type: "object", properties: {}, required: [] } })).toEqual([]);
  });
  it("handles a non-object / undefined inputSchema without throwing", () => {
    expect(computeToolLints({ name: "x", description: "Fetch a pet", inputSchema: undefined })).toEqual([]);
    expect(computeToolLints({ name: "x", description: "Fetch a pet", inputSchema: "nonsense" })).toEqual([]);
  });
});
