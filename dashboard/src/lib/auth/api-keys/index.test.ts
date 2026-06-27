import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  getApiKeyConfig,
  getApiKeyStore,
  createUserFromApiKey,
  warnIfMissingOwnerSnapshot,
  resetApiKeyAuthWarningsForTest,
} from "./index";
import type { ApiKey } from "./types";
import { PostgresApiKeyStore } from "./postgres-store";
import { FileApiKeyStore } from "./file-store";
import { MemoryApiKeyStore } from "./memory-store";

const saved = { ...process.env };
afterEach(() => {
  process.env = { ...saved };
  vi.restoreAllMocks();
});
beforeEach(() => {
  delete process.env.OMNIA_AUTH_API_KEYS_STORE;
  delete process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL;
  delete process.env.OMNIA_BUILTIN_POSTGRES_URL;
  resetApiKeyAuthWarningsForTest();
});

describe("getApiKeyConfig", () => {
  it("defaults storeType to memory", () => {
    expect(getApiKeyConfig().storeType).toBe("memory");
  });
  it("resolves postgresUrl from the api-keys URL", () => {
    process.env.OMNIA_AUTH_API_KEYS_STORE = "postgres";
    process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL = "postgres://api/keys";
    expect(getApiKeyConfig().postgresUrl).toBe("postgres://api/keys");
  });
  it("falls back to the builtin postgres URL when the api-keys URL is unset", () => {
    process.env.OMNIA_AUTH_API_KEYS_STORE = "postgres";
    process.env.OMNIA_BUILTIN_POSTGRES_URL = "postgres://builtin/users";
    expect(getApiKeyConfig().postgresUrl).toBe("postgres://builtin/users");
  });
  it("allows key creation for memory and postgres, but not file", () => {
    expect(getApiKeyConfig().allowCreate).toBe(true); // default memory
    process.env.OMNIA_AUTH_API_KEYS_STORE = "postgres";
    expect(getApiKeyConfig().allowCreate).toBe(true);
    process.env.OMNIA_AUTH_API_KEYS_STORE = "file";
    expect(getApiKeyConfig().allowCreate).toBe(false);
  });
});

describe("getApiKeyStore dispatch", () => {
  it("returns the memory store by default", () => {
    expect(getApiKeyStore()).toBeInstanceOf(MemoryApiKeyStore);
  });
  it("returns the file store when store=file", () => {
    process.env.OMNIA_AUTH_API_KEYS_STORE = "file";
    process.env.OMNIA_AUTH_API_KEYS_FILE_PATH = "/tmp/omnia-test-keys.json";
    expect(getApiKeyStore()).toBeInstanceOf(FileApiKeyStore);
  });
  it("returns the postgres store when store=postgres with a URL", () => {
    process.env.OMNIA_AUTH_API_KEYS_STORE = "postgres";
    process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL = "postgres://api/keys";
    expect(getApiKeyStore()).toBeInstanceOf(PostgresApiKeyStore);
  });
});

describe("createUserFromApiKey owner snapshot", () => {
  const base: ApiKey = {
    id: "k1", userId: "u1", name: "ci", keyPrefix: "omnia_sk_x...",
    keyHash: "h", role: "editor", expiresAt: null, createdAt: new Date(), lastUsedAt: null,
  };

  it("carries the owner snapshot + workspace scope onto the User", () => {
    const u = createUserFromApiKey({
      ...base,
      ownerEmail: "alice@example.com",
      ownerGroups: ["devs@example.com"],
      workspaces: ["demo"],
    });
    expect(u.email).toBe("alice@example.com");
    expect(u.groups).toEqual(["devs@example.com"]);
    expect(u.apiKeyScope).toEqual({ workspaces: ["demo"] });
    expect(u.provider).toBe("proxy");
  });

  it("is backward compatible for a key with no snapshot", () => {
    const u = createUserFromApiKey(base);
    expect(u.email).toBeUndefined();
    expect(u.groups).toEqual([]);
    expect(u.apiKeyScope).toEqual({ workspaces: undefined });
  });
});

describe("getApiKeyStore memory-store-with-postgres-url warning (#1582)", () => {
  it("warns once when the memory store is used while a postgres URL is wired", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL = "postgres://api/keys";
    // store unset → defaults to memory, but a durable URL is present.
    getApiKeyStore();
    getApiKeyStore();
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0][0]).toMatch(/ephemeral in-memory store/);
  });

  it("does not warn for the memory store when no postgres URL is set", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    getApiKeyStore();
    expect(warn).not.toHaveBeenCalled();
  });

  it("does not warn when the postgres store is actually selected", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    process.env.OMNIA_AUTH_API_KEYS_STORE = "postgres";
    process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL = "postgres://api/keys";
    getApiKeyStore();
    expect(warn).not.toHaveBeenCalled();
  });
});

describe("warnIfMissingOwnerSnapshot (#1582)", () => {
  const base: ApiKey = {
    id: "k1", userId: "u1", name: "deploy-demo", keyPrefix: "omnia_sk_x...",
    keyHash: "h", role: "viewer", expiresAt: null, createdAt: new Date(), lastUsedAt: null,
  };

  it("warns for a workspace-scoped key with no owner snapshot", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    warnIfMissingOwnerSnapshot({ ...base, workspaces: ["demo"] });
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0][0]).toMatch(/no owner snapshot/);
  });

  it("does not warn when a scoped key carries owner groups", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    warnIfMissingOwnerSnapshot({ ...base, workspaces: ["demo"], ownerGroups: ["c16e8ed8"] });
    expect(warn).not.toHaveBeenCalled();
  });

  it("does not warn when a scoped key carries an owner email", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    warnIfMissingOwnerSnapshot({ ...base, workspaces: ["demo"], ownerEmail: "a@b.com" });
    expect(warn).not.toHaveBeenCalled();
  });

  it("does not warn for an unscoped legacy key with no snapshot", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    warnIfMissingOwnerSnapshot(base);
    expect(warn).not.toHaveBeenCalled();
  });
});
