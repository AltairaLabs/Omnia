/**
 * @vitest-environment jsdom
 */
import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  registerClientToolHandler,
  getClientToolHandler,
  hasClientToolHandler,
  isAutoApproved,
  setAutoApproved,
  clearAutoApproved,
} from "./client-tools";

describe("client tool handler registry", () => {
  it("registers and retrieves a handler", () => {
    const handler = vi.fn().mockResolvedValue({ result: "ok" });
    registerClientToolHandler("test_tool", handler);
    expect(getClientToolHandler("test_tool")).toBe(handler);
    expect(hasClientToolHandler("test_tool")).toBe(true);
  });

  it("returns undefined for unregistered tool", () => {
    expect(getClientToolHandler("nonexistent")).toBeUndefined();
    expect(hasClientToolHandler("nonexistent")).toBe(false);
  });

  it("has get_user_location registered by default", () => {
    expect(hasClientToolHandler("get_user_location")).toBe(true);
  });

  it("get_user_location rejects when geolocation is unavailable", async () => {
    const handler = getClientToolHandler("get_user_location")!;
    // jsdom doesn't have navigator.geolocation by default
    const originalGeo = navigator.geolocation;
    Object.defineProperty(navigator, "geolocation", { value: undefined, configurable: true });
    await expect(handler()).rejects.toThrow("Geolocation is not available");
    Object.defineProperty(navigator, "geolocation", { value: originalGeo, configurable: true });
  });

  it("overwrites an existing handler", () => {
    const handler1 = vi.fn().mockResolvedValue("first");
    const handler2 = vi.fn().mockResolvedValue("second");
    registerClientToolHandler("overwrite_test", handler1);
    registerClientToolHandler("overwrite_test", handler2);
    expect(getClientToolHandler("overwrite_test")).toBe(handler2);
  });
});

describe("auto-approve persistence", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("is not auto-approved by default", () => {
    expect(isAutoApproved("some_tool")).toBe(false);
  });

  it("persists auto-approval", () => {
    setAutoApproved("some_tool");
    expect(isAutoApproved("some_tool")).toBe(true);
  });

  it("clears auto-approval", () => {
    setAutoApproved("some_tool");
    clearAutoApproved("some_tool");
    expect(isAutoApproved("some_tool")).toBe(false);
  });

  it("handles corrupted localStorage gracefully", () => {
    localStorage.setItem("omnia-client-tool-auto-approve", "not-valid-json");
    expect(isAutoApproved("anything")).toBe(false);
  });

  it("setAutoApproved is idempotent", () => {
    setAutoApproved("tool_x");
    setAutoApproved("tool_x");
    expect(isAutoApproved("tool_x")).toBe(true);
    // Clear and verify only removed once
    clearAutoApproved("tool_x");
    expect(isAutoApproved("tool_x")).toBe(false);
  });

  it("clearAutoApproved on non-existent tool is a no-op", () => {
    clearAutoApproved("never_added");
    expect(isAutoApproved("never_added")).toBe(false);
  });

  it("handles multiple tools", () => {
    setAutoApproved("tool_a");
    setAutoApproved("tool_b");
    expect(isAutoApproved("tool_a")).toBe(true);
    expect(isAutoApproved("tool_b")).toBe(true);
    clearAutoApproved("tool_a");
    expect(isAutoApproved("tool_a")).toBe(false);
    expect(isAutoApproved("tool_b")).toBe(true);
  });
});
