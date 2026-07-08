import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";
import { GET, POST } from "./route";

const mockGetUser = vi.fn();
const mockUserHasPermission = vi.fn();
const mockGetEffectiveLicense = vi.fn();
const mockWriteLicenseSecret = vi.fn();

vi.mock("@/lib/auth", () => ({
  getUser: () => mockGetUser(),
}));

vi.mock("@/lib/auth/permissions", () => ({
  Permission: { SETTINGS_EDIT: "settings:edit" },
  userHasPermission: (user: unknown, permission: string) =>
    mockUserHasPermission(user, permission),
}));

vi.mock("@/lib/license/resolve-server", () => ({
  getEffectiveLicense: () => mockGetEffectiveLicense(),
}));

// Keep parseLicenseJwt real; only stub the k8s write.
vi.mock("@/lib/k8s/license-secret", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/license-secret")>();
  return {
    ...actual,
    writeLicenseSecret: (...args: unknown[]) => mockWriteLicenseSecret(...args),
  };
});

function makeJwt(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    Buffer.from(JSON.stringify(obj))
      .toString("base64")
      .replaceAll("+", "-")
      .replaceAll("/", "_")
      .replaceAll("=", "");
  return `${b64url({ alg: "RS256" })}.${b64url(payload)}.sig`;
}

function uploadRequest(fileContent: string | null): NextRequest {
  const form = new FormData();
  if (fileContent !== null) {
    form.append("license", new File([fileContent], "license.jwt"));
  }
  return new NextRequest("http://localhost/api/license", {
    method: "POST",
    body: form,
  });
}

const VALID_JWT = makeJwt({ tier: "enterprise", customer: "Acme", exp: 1893456000 });

describe("GET /api/license", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns the resolved license", async () => {
    mockGetEffectiveLicense.mockResolvedValue({ tier: "enterprise" });
    const response = await GET();
    expect(await response.json()).toEqual({ tier: "enterprise" });
  });
});

describe("POST /api/license", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetUser.mockResolvedValue({ id: "admin-1" });
    mockUserHasPermission.mockReturnValue(true);
    mockWriteLicenseSecret.mockResolvedValue(undefined);
  });

  it("stores a valid license and echoes decoded claims", async () => {
    const response = await POST(uploadRequest(VALID_JWT));
    expect(response.status).toBe(200);
    const data = await response.json();
    expect(data.tier).toBe("enterprise");
    expect(data.customer).toBe("Acme");
    expect(data.message).toMatch(/5 minutes/);
    expect(mockWriteLicenseSecret).toHaveBeenCalledWith(VALID_JWT);
  });

  it("trims surrounding whitespace before storing", async () => {
    await POST(uploadRequest(`\n  ${VALID_JWT}  \n`));
    expect(mockWriteLicenseSecret).toHaveBeenCalledWith(VALID_JWT);
  });

  it("accepts the license as a plain string form field", async () => {
    const form = new FormData();
    form.append("license", VALID_JWT);
    const request = new NextRequest("http://localhost/api/license", {
      method: "POST",
      body: form,
    });
    const response = await POST(request);
    expect(response.status).toBe(200);
    expect(mockWriteLicenseSecret).toHaveBeenCalledWith(VALID_JWT);
  });

  it("returns 400 when the uploaded file is blank", async () => {
    const response = await POST(uploadRequest("   \n  "));
    expect(response.status).toBe(400);
    expect((await response.json()).error).toMatch(/No license/);
    expect(mockWriteLicenseSecret).not.toHaveBeenCalled();
  });

  it("rejects when the caller lacks settings:edit", async () => {
    mockUserHasPermission.mockReturnValue(false);
    const response = await POST(uploadRequest(VALID_JWT));
    expect(response.status).toBe(403);
    expect(mockWriteLicenseSecret).not.toHaveBeenCalled();
  });

  it("returns 400 when no license field is present", async () => {
    const response = await POST(uploadRequest(null));
    expect(response.status).toBe(400);
    expect((await response.json()).error).toMatch(/No license/);
    expect(mockWriteLicenseSecret).not.toHaveBeenCalled();
  });

  it("returns 400 for a malformed license", async () => {
    const response = await POST(uploadRequest("not-a-jwt"));
    expect(response.status).toBe(400);
    expect((await response.json()).error).toMatch(/3 dot-separated/);
    expect(mockWriteLicenseSecret).not.toHaveBeenCalled();
  });

  it("returns 500 when the Secret write fails", async () => {
    mockWriteLicenseSecret.mockRejectedValue(new Error("k8s down"));
    const response = await POST(uploadRequest(VALID_JWT));
    expect(response.status).toBe(500);
  });
});
