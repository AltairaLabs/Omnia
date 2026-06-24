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
