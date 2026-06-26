import { describe, it, expect, vi, beforeEach } from "vitest";
import { RedisSessionStore } from "./redis-store";
import type { CliCodeRecord, CliFlowRecord, PkceRecord, SessionRecord } from "./types";

function makeRedis() {
  return {
    get: vi.fn(),
    set: vi.fn(),
    del: vi.fn(),
    // ioredis exposes GETDEL as `getdel` on the client.
    getdel: vi.fn(),
  };
}

const sampleSession: SessionRecord = {
  user: { id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth" },
  oauth: { provider: "azure", refreshToken: "r", idToken: "i", expiresAt: 1 },
  createdAt: 1000,
};

const samplePkce: PkceRecord = {
  codeVerifier: "v", codeChallenge: "c", state: "s", returnTo: "/x", createdAt: 1000,
};

describe("RedisSessionStore", () => {
  let redis: ReturnType<typeof makeRedis>;
  let store: RedisSessionStore;

  beforeEach(() => {
    redis = makeRedis();
    store = new RedisSessionStore(redis as unknown as import("ioredis").default);
  });

  describe("getSession", () => {
    it("returns null when Redis has no value", async () => {
      redis.get.mockResolvedValue(null);
      expect(await store.getSession("sid")).toBeNull();
      expect(redis.get).toHaveBeenCalledWith("omnia:sess:sid");
    });

    it("returns the parsed record when present", async () => {
      redis.get.mockResolvedValue(JSON.stringify(sampleSession));
      expect(await store.getSession("sid")).toEqual(sampleSession);
    });

    it("returns null and logs when stored JSON is corrupt", async () => {
      redis.get.mockResolvedValue("not-json");
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      expect(await store.getSession("sid")).toBeNull();
      expect(spy).toHaveBeenCalled();
      spy.mockRestore();
    });
  });

  describe("putSession", () => {
    it("SETs the key with EX ttl", async () => {
      await store.putSession("sid", sampleSession, 3600);
      expect(redis.set).toHaveBeenCalledWith(
        "omnia:sess:sid",
        JSON.stringify(sampleSession),
        "EX",
        3600,
      );
    });

    it("rejects non-positive TTLs", async () => {
      await expect(store.putSession("sid", sampleSession, 0)).rejects.toThrow();
    });
  });

  describe("deleteSession", () => {
    it("DELs the key", async () => {
      await store.deleteSession("sid");
      expect(redis.del).toHaveBeenCalledWith("omnia:sess:sid");
    });
  });

  describe("putPkce", () => {
    it("SETs the pkce key with EX ttl", async () => {
      await store.putPkce("state", samplePkce, 300);
      expect(redis.set).toHaveBeenCalledWith(
        "omnia:pkce:state",
        JSON.stringify(samplePkce),
        "EX",
        300,
      );
    });
  });

  describe("consumePkce", () => {
    it("uses GETDEL and returns the parsed record", async () => {
      redis.getdel.mockResolvedValue(JSON.stringify(samplePkce));
      expect(await store.consumePkce("state")).toEqual(samplePkce);
      expect(redis.getdel).toHaveBeenCalledWith("omnia:pkce:state");
    });

    it("returns null when the key is missing", async () => {
      redis.getdel.mockResolvedValue(null);
      expect(await store.consumePkce("state")).toBeNull();
    });

    it("returns null and logs when stored JSON is corrupt", async () => {
      redis.getdel.mockResolvedValue("not-json");
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      expect(await store.consumePkce("state")).toBeNull();
      expect(spy).toHaveBeenCalled();
      spy.mockRestore();
    });
  });

  // Additional tests for coverage
  describe("putPkce ttl validation", () => {
    it("rejects non-positive TTLs for pkce", async () => {
      await expect(store.putPkce("state", samplePkce, 0)).rejects.toThrow();
      await expect(store.putPkce("state", samplePkce, -1)).rejects.toThrow();
    });
  });

  describe("putSession ttl validation", () => {
    it("rejects negative TTLs", async () => {
      await expect(store.putSession("sid", sampleSession, -1)).rejects.toThrow();
    });
  });

  const sampleCliFlow: CliFlowRecord = {
    callback: "http://127.0.0.1:5000/cb",
    cliState: "abcd1234",
    createdAt: 1000,
  };

  const sampleCliCode: CliCodeRecord = {
    userId: "u1",
    email: "u@e.com",
    groups: ["g"],
    userRole: "editor",
    workspace: "team-acme",
    workspaceRole: "editor",
    createdAt: 1000,
  };

  describe("putCliFlow", () => {
    it("SETs the cliflow key with EX ttl", async () => {
      await store.putCliFlow("flow1", sampleCliFlow, 300);
      expect(redis.set).toHaveBeenCalledWith(
        "omnia:cliflow:flow1",
        JSON.stringify(sampleCliFlow),
        "EX",
        300,
      );
    });

    it("rejects non-positive TTLs", async () => {
      await expect(store.putCliFlow("flow1", sampleCliFlow, 0)).rejects.toThrow();
      await expect(store.putCliFlow("flow1", sampleCliFlow, -1)).rejects.toThrow();
    });
  });

  describe("getCliFlow", () => {
    it("returns the parsed record when present", async () => {
      redis.get.mockResolvedValue(JSON.stringify(sampleCliFlow));
      expect(await store.getCliFlow("flow1")).toEqual(sampleCliFlow);
      expect(redis.get).toHaveBeenCalledWith("omnia:cliflow:flow1");
    });

    it("returns null when the key is missing", async () => {
      redis.get.mockResolvedValue(null);
      expect(await store.getCliFlow("flow1")).toBeNull();
    });
  });

  describe("consumeCliFlow", () => {
    it("uses GETDEL and returns the parsed record", async () => {
      redis.getdel.mockResolvedValue(JSON.stringify(sampleCliFlow));
      expect(await store.consumeCliFlow("flow1")).toEqual(sampleCliFlow);
      expect(redis.getdel).toHaveBeenCalledWith("omnia:cliflow:flow1");
    });

    it("returns null when the key is missing", async () => {
      redis.getdel.mockResolvedValue(null);
      expect(await store.consumeCliFlow("flow1")).toBeNull();
    });
  });

  describe("putCliCode", () => {
    it("SETs the clicode key with EX ttl", async () => {
      await store.putCliCode("code1", sampleCliCode, 60);
      expect(redis.set).toHaveBeenCalledWith(
        "omnia:clicode:code1",
        JSON.stringify(sampleCliCode),
        "EX",
        60,
      );
    });

    it("rejects non-positive TTLs", async () => {
      await expect(store.putCliCode("code1", sampleCliCode, 0)).rejects.toThrow();
      await expect(store.putCliCode("code1", sampleCliCode, -1)).rejects.toThrow();
    });
  });

  describe("consumeCliCode", () => {
    it("uses GETDEL and returns the parsed record", async () => {
      redis.getdel.mockResolvedValue(JSON.stringify(sampleCliCode));
      expect(await store.consumeCliCode("code1")).toEqual(sampleCliCode);
      expect(redis.getdel).toHaveBeenCalledWith("omnia:clicode:code1");
    });

    it("returns null when the key is missing", async () => {
      redis.getdel.mockResolvedValue(null);
      expect(await store.consumeCliCode("code1")).toBeNull();
    });
  });
});
