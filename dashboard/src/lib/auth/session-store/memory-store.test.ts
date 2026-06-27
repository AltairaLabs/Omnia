import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { MemorySessionStore } from "./memory-store";
import type { SessionRecord, PkceRecord, CliFlowRecord, CliCodeRecord } from "./types";

const SID = "sid-abc";
const STATE = "state-xyz";

const sampleSession: SessionRecord = {
  user: {
    id: "u1",
    username: "u1",
    groups: [],
    role: "viewer",
    provider: "oauth",
  },
  oauth: { provider: "azure", refreshToken: "r", idToken: "i", expiresAt: 1 },
  createdAt: 1000,
};

const samplePkce: PkceRecord = {
  codeVerifier: "v",
  codeChallenge: "c",
  state: STATE,
  returnTo: "/dash",
  createdAt: 1000,
};

describe("MemorySessionStore", () => {
  let store: MemorySessionStore;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(1_000_000));
    store = new MemorySessionStore();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns null for an unknown session id", async () => {
    expect(await store.getSession(SID)).toBeNull();
  });

  it("round-trips a session record within its TTL", async () => {
    await store.putSession(SID, sampleSession, 60);
    expect(await store.getSession(SID)).toEqual(sampleSession);
  });

  it("expires a session record after its TTL", async () => {
    await store.putSession(SID, sampleSession, 30);
    vi.advanceTimersByTime(31_000);
    expect(await store.getSession(SID)).toBeNull();
  });

  it("deleteSession removes the record", async () => {
    await store.putSession(SID, sampleSession, 60);
    await store.deleteSession(SID);
    expect(await store.getSession(SID)).toBeNull();
  });

  it("deleteSession on a missing key is a no-op", async () => {
    await expect(store.deleteSession("missing")).resolves.toBeUndefined();
  });

  it("consumePkce returns null for unknown state", async () => {
    expect(await store.consumePkce(STATE)).toBeNull();
  });

  it("consumePkce returns the record exactly once", async () => {
    await store.putPkce(STATE, samplePkce, 60);
    const first = await store.consumePkce(STATE);
    const second = await store.consumePkce(STATE);
    expect(first).toEqual(samplePkce);
    expect(second).toBeNull();
  });

  it("consumePkce returns null when the record has expired", async () => {
    await store.putPkce(STATE, samplePkce, 5);
    vi.advanceTimersByTime(6_000);
    expect(await store.consumePkce(STATE)).toBeNull();
  });

  it("rejects non-positive TTLs for sessions", async () => {
    await expect(store.putSession(SID, sampleSession, 0)).rejects.toThrow();
    await expect(store.putSession(SID, sampleSession, -1)).rejects.toThrow();
  });

  it("rejects non-positive TTLs for pkce", async () => {
    await expect(store.putPkce(STATE, samplePkce, 0)).rejects.toThrow();
  });
});

const flow: CliFlowRecord = { callback: "http://127.0.0.1:5000/cb", cliState: "abc123xy", createdAt: 1 };
const codeRec: CliCodeRecord = {
  userId: "u1", email: "u@e.com", groups: ["g"], userRole: "editor",
  workspace: "team-acme", workspaceRole: "editor", createdAt: 1,
};

describe("CLI flow + code", () => {
  it("peeks a flow without consuming it, then consumes it once", async () => {
    const s = new MemorySessionStore();
    await s.putCliFlow("f1", flow, 60);
    expect(await s.getCliFlow("f1")).toEqual(flow);
    expect(await s.getCliFlow("f1")).toEqual(flow); // peek is repeatable
    expect(await s.consumeCliFlow("f1")).toEqual(flow);
    expect(await s.getCliFlow("f1")).toBeNull(); // gone after consume
  });

  it("consumes a code exactly once (replay → null)", async () => {
    const s = new MemorySessionStore();
    await s.putCliCode("c1", codeRec, 60);
    expect(await s.consumeCliCode("c1")).toEqual(codeRec);
    expect(await s.consumeCliCode("c1")).toBeNull();
  });

  it("returns null for an expired code", async () => {
    const s = new MemorySessionStore();
    await s.putCliCode("c2", codeRec, 60);
    vi.useFakeTimers();
    vi.advanceTimersByTime(61_000);
    expect(await s.consumeCliCode("c2")).toBeNull();
    vi.useRealTimers();
  });
});
