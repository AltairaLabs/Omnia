import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/permissions", () => ({
  Permission: { API_KEYS_VIEW_OWN: "v", API_KEYS_MANAGE_OWN: "m" },
  userHasPermission: () => true,
}));
const create = vi.fn(async () => ({ id: "k1", key: "omnia_sk_x" }));
const listByUser = vi.fn(async () => []);
vi.mock("@/lib/auth/api-keys", () => ({
  isApiKeyAuthEnabled: () => true,
  getApiKeyConfig: () => ({ allowCreate: true, maxKeysPerUser: 10, defaultExpirationDays: 90, storeType: "postgres" }),
  getApiKeyStore: () => ({ create, listByUser }),
}));

import { POST } from "./route";
import { getUser } from "@/lib/auth";

beforeEach(() => {
  vi.clearAllMocks();
  (getUser as ReturnType<typeof vi.fn>).mockResolvedValue({
    id: "u1", username: "alice", email: "alice@example.com",
    groups: ["devs"], role: "editor", provider: "oauth",
  });
});

function postReq(body: unknown): Request {
  return new Request("https://localhost/api/settings/api-keys", {
    method: "POST", body: JSON.stringify(body),
  });
}

describe("POST /api/settings/api-keys owner snapshot", () => {
  it("snapshots the session user's email + groups and passes workspaces", async () => {
    const res = await POST(postReq({ name: "ci", workspaces: ["demo"] }) as never);
    expect(res.status).toBe(201);
    expect(create).toHaveBeenCalledWith("u1", expect.objectContaining({
      name: "ci", ownerEmail: "alice@example.com", ownerGroups: ["devs"], workspaces: ["demo"],
    }));
  });

  it("omits workspaces when not provided (unrestricted)", async () => {
    await POST(postReq({ name: "global" }) as never);
    expect(create).toHaveBeenCalledWith("u1", expect.objectContaining({ workspaces: undefined }));
  });
});
