import { describe, it, expect } from "vitest";
import { MemoryApiKeyStore } from "./memory-store";

describe("MemoryApiKeyStore workspaces round-trip", () => {
  it("persists and returns the workspaces allowlist", async () => {
    const store = new MemoryApiKeyStore();
    const created = await store.create("user-1", { name: "ci", workspaces: ["demo", "staging"] });
    expect(created.workspaces).toEqual(["demo", "staging"]);

    const found = await store.findByKey(created.key);
    expect(found?.workspaces).toEqual(["demo", "staging"]);

    const listed = await store.listByUser("user-1");
    expect(listed[0].workspaces).toEqual(["demo", "staging"]);
  });

  it("leaves workspaces undefined when not provided (unrestricted)", async () => {
    const store = new MemoryApiKeyStore();
    const created = await store.create("user-1", { name: "global" });
    expect(created.workspaces).toBeUndefined();
    const found = await store.findByKey(created.key);
    expect(found?.workspaces).toBeUndefined();
  });
});

describe("MemoryApiKeyStore owner snapshot round-trip", () => {
  it("persists ownerEmail + ownerGroups", async () => {
    const store = new MemoryApiKeyStore();
    const created = await store.create("u1", {
      name: "ci", ownerEmail: "alice@example.com", ownerGroups: ["devs"],
    });
    const found = await store.findByKey(created.key);
    expect(found?.ownerEmail).toBe("alice@example.com");
    expect(found?.ownerGroups).toEqual(["devs"]);
  });
});

describe("expiresInSeconds", () => {
  it("sets a sub-day expiry and takes precedence over expiresInDays", async () => {
    const store = new MemoryApiKeyStore();
    const before = Date.now();
    const created = await store.create("u1", {
      name: "cli",
      expiresInSeconds: 3600,
      expiresInDays: 90, // must be ignored when seconds is set
    });
    const ms = created.expiresAt!.getTime() - before;
    expect(ms).toBeGreaterThan(3500 * 1000);
    expect(ms).toBeLessThan(3700 * 1000);
  });

  it("ignores a non-positive expiresInSeconds and falls back to days", async () => {
    const store = new MemoryApiKeyStore();
    const created = await store.create("u1", { name: "x", expiresInSeconds: 0, expiresInDays: 1 });
    const hours = (created.expiresAt!.getTime() - Date.now()) / 3_600_000;
    expect(hours).toBeGreaterThan(23);
    expect(hours).toBeLessThan(25);
  });
});
