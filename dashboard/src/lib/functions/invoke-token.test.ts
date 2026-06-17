import { describe, it, expect, afterEach } from "vitest";
import { generateKeyPairSync } from "node:crypto";
import { writeFileSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { mgmtPlaneAuthHeaders } from "./invoke-token";

describe("mgmtPlaneAuthHeaders", () => {
  const prev = process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;

  afterEach(() => {
    if (prev === undefined) delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    else process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = prev;
  });

  it("returns no Authorization header when no signing key is configured (dev fallback)", () => {
    delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    expect(mgmtPlaneAuthHeaders("deep-research", "dev-agents", "admin")).toEqual({});
  });

  it("returns no header when the signing key path is unreadable", () => {
    process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = "/nonexistent/key.pem";
    expect(mgmtPlaneAuthHeaders("deep-research", "dev-agents", "admin")).toEqual({});
  });

  it("mints a Bearer token when a valid RSA signing key is configured", () => {
    const { privateKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
    const pem = privateKey.export({ type: "pkcs8", format: "pem" }) as string;
    const dir = mkdtempSync(join(tmpdir(), "invoke-token-"));
    const keyPath = join(dir, "signing-key.pem");
    writeFileSync(keyPath, pem);
    process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = keyPath;

    const headers = mgmtPlaneAuthHeaders("deep-research", "dev-agents", "admin");
    expect(headers.Authorization).toMatch(/^Bearer [\w-]+\.[\w-]+\.[\w-]+$/);
  });
});
