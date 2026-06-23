import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { getApiKeyConfig, getApiKeyStore } from "./index";
import { PostgresApiKeyStore } from "./postgres-store";
import { FileApiKeyStore } from "./file-store";
import { MemoryApiKeyStore } from "./memory-store";

const saved = { ...process.env };
afterEach(() => { process.env = { ...saved }; });
beforeEach(() => {
  delete process.env.OMNIA_AUTH_API_KEYS_STORE;
  delete process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL;
  delete process.env.OMNIA_BUILTIN_POSTGRES_URL;
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
