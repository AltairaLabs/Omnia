import { describe, it, expect, vi, beforeEach } from "vitest";
import { getDeviceId } from "./device-id";

describe("getDeviceId", () => {
  let store: Record<string, string>;

  beforeEach(() => {
    store = {};
    vi.stubGlobal("localStorage", {
      getItem: vi.fn((key: string) => store[key] ?? null),
      setItem: vi.fn((key: string, value: string) => {
        store[key] = value;
      }),
    });
  });

  it("generates a UUID and stores it", () => {
    const id = getDeviceId();
    expect(id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
    );
    expect(localStorage.setItem).toHaveBeenCalledWith("omnia-device-id", id);
  });

  it("returns the same ID on subsequent calls", () => {
    const first = getDeviceId();
    const second = getDeviceId();
    expect(second).toBe(first);
  });

  it("returns empty string when localStorage is unavailable", () => {
    vi.stubGlobal("localStorage", undefined);
    expect(getDeviceId()).toBe("");
  });
});
