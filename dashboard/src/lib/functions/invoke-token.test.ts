import { describe, it, expect, afterEach } from "vitest";
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
});
