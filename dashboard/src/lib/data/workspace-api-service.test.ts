import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { WorkspaceApiService } from "./workspace-api-service";

describe("WorkspaceApiService.getDeployProfile", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("GETs the deploy-profile endpoint and returns the payload", async () => {
    const payload = { api_endpoint: "https://o", workspace: "w", providers: [], skills: [] };
    vi.mocked(global.fetch).mockResolvedValue({
      ok: true,
      json: async () => payload,
    } as Response);
    const svc = new WorkspaceApiService();
    const result = await svc.getDeployProfile("team acme");
    expect(global.fetch).toHaveBeenCalledWith("/api/workspaces/team%20acme/deploy-profile");
    expect(result).toEqual(payload);
  });

  it("throws on non-ok response", async () => {
    vi.mocked(global.fetch).mockResolvedValue({ ok: false, statusText: "Boom" } as Response);
    const svc = new WorkspaceApiService();
    await expect(svc.getDeployProfile("w")).rejects.toThrow("Boom");
  });
});
