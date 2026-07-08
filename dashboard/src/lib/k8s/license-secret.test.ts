import { describe, it, expect, vi } from "vitest";
import type * as k8s from "@kubernetes/client-node";
import {
  parseLicenseJwt,
  writeLicenseSecret,
  LICENSE_SECRET_NAME,
  LICENSE_SECRET_KEY,
} from "./license-secret";

// Shared stub for the default (non-injected) client path. vi.hoisted so the
// vi.mock factory can close over it.
const { defaultApi } = vi.hoisted(() => ({
  defaultApi: {
    readNamespacedSecret: vi.fn(),
    replaceNamespacedSecret: vi.fn(),
    createNamespacedSecret: vi.fn(),
  },
}));

vi.mock("@kubernetes/client-node", () => ({
  KubeConfig: class {
    loadFromCluster() {
      throw new Error("not in cluster");
    }
    loadFromDefault() {}
    makeApiClient() {
      return defaultApi;
    }
  },
  CoreV1Api: class {},
}));

/** Build a JWT-shaped string (header.payload.signature) from a claims object. */
function makeJwt(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    Buffer.from(JSON.stringify(obj))
      .toString("base64")
      .replaceAll("+", "-")
      .replaceAll("/", "_")
      .replaceAll("=", "");
  return `${b64url({ alg: "RS256", typ: "JWT" })}.${b64url(payload)}.sig`;
}

describe("parseLicenseJwt", () => {
  it("decodes tier, customer, and expiry from a valid token", () => {
    const exp = 1893456000; // 2030-01-01
    const token = makeJwt({ tier: "enterprise", customer: "Acme", exp });
    const claims = parseLicenseJwt(token);
    expect(claims.tier).toBe("enterprise");
    expect(claims.customer).toBe("Acme");
    expect(claims.expiresAt).toBe(new Date(exp * 1000).toISOString());
  });

  it("tolerates surrounding whitespace", () => {
    const token = makeJwt({ tier: "trial", customer: "X", exp: 1893456000 });
    expect(parseLicenseJwt(`\n  ${token}\n`).tier).toBe("trial");
  });

  it("defaults customer to empty and expiresAt to null when absent", () => {
    const claims = parseLicenseJwt(makeJwt({ tier: "enterprise" }));
    expect(claims.customer).toBe("");
    expect(claims.expiresAt).toBeNull();
  });

  it("rejects an empty string", () => {
    expect(() => parseLicenseJwt("   ")).toThrow(/empty/i);
  });

  it("rejects a token without three segments", () => {
    expect(() => parseLicenseJwt("not.a.valid.jwt")).toThrow(/3 dot-separated/);
    expect(() => parseLicenseJwt("onlyone")).toThrow(/3 dot-separated/);
  });

  it("rejects a payload that is not decodable JSON", () => {
    expect(() => parseLicenseJwt("aaa.%%%.bbb")).toThrow(/decodable JSON/);
  });

  it("rejects a token missing the tier claim", () => {
    expect(() => parseLicenseJwt(makeJwt({ customer: "Acme" }))).toThrow(/tier/);
  });
});

interface FakeApi {
  readNamespacedSecret: ReturnType<typeof vi.fn>;
  replaceNamespacedSecret: ReturnType<typeof vi.fn>;
  createNamespacedSecret: ReturnType<typeof vi.fn>;
}

function fakeApi(): FakeApi {
  return {
    readNamespacedSecret: vi.fn(),
    replaceNamespacedSecret: vi.fn(),
    createNamespacedSecret: vi.fn(),
  };
}

describe("writeLicenseSecret", () => {
  const jwt = "header.payload.sig";
  const expectedData = { [LICENSE_SECRET_KEY]: Buffer.from(jwt).toString("base64") };

  it("replaces an existing Secret, carrying its resourceVersion", async () => {
    const api = fakeApi();
    api.readNamespacedSecret.mockResolvedValue({
      metadata: { resourceVersion: "42" },
    });

    await writeLicenseSecret(jwt, api as unknown as k8s.CoreV1Api);

    expect(api.replaceNamespacedSecret).toHaveBeenCalledTimes(1);
    expect(api.createNamespacedSecret).not.toHaveBeenCalled();
    const body = api.replaceNamespacedSecret.mock.calls[0][0].body;
    expect(body.metadata.name).toBe(LICENSE_SECRET_NAME);
    expect(body.metadata.resourceVersion).toBe("42");
    expect(body.type).toBe("Opaque");
    expect(body.data).toEqual(expectedData);
  });

  it("creates the Secret when it does not exist (404)", async () => {
    const api = fakeApi();
    api.readNamespacedSecret.mockRejectedValue({ statusCode: 404 });

    await writeLicenseSecret(jwt, api as unknown as k8s.CoreV1Api);

    expect(api.createNamespacedSecret).toHaveBeenCalledTimes(1);
    expect(api.replaceNamespacedSecret).not.toHaveBeenCalled();
    const body = api.createNamespacedSecret.mock.calls[0][0].body;
    expect(body.metadata.name).toBe(LICENSE_SECRET_NAME);
    expect(body.data).toEqual(expectedData);
  });

  it("propagates non-404 read errors without writing", async () => {
    const api = fakeApi();
    api.readNamespacedSecret.mockRejectedValue({ statusCode: 500 });

    await expect(
      writeLicenseSecret(jwt, api as unknown as k8s.CoreV1Api)
    ).rejects.toEqual({ statusCode: 500 });
    expect(api.createNamespacedSecret).not.toHaveBeenCalled();
    expect(api.replaceNamespacedSecret).not.toHaveBeenCalled();
  });

  it("builds the default in-cluster client when none is injected", async () => {
    defaultApi.readNamespacedSecret.mockResolvedValue({
      metadata: { resourceVersion: "7" },
    });

    await writeLicenseSecret(jwt);

    expect(defaultApi.replaceNamespacedSecret).toHaveBeenCalledTimes(1);
  });
});
