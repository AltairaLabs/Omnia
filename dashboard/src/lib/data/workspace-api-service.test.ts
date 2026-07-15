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

// Mock-to-contract: the promptpacks collection GET returns a BARE ARRAY of
// PromptPack objects (crd-route-factory `NextResponse.json(items)`), not
// `{ items: [...] }`.
function makePack(name: string, packName: string, version: string) {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: { name, namespace: "ns1" },
    spec: {
      packName,
      source: { type: "configmap", configMapRef: { name: "cm" } },
      version,
    },
  };
}

describe("WorkspaceApiService.getPromptPack", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("lists by the packName label selector and returns the channel-max stable version", async () => {
    const packs = [
      makePack("pp-v100", "rag-hero", "1.0.0"),
      makePack("pp-v110", "rag-hero", "1.1.0"),
      makePack("pp-v200beta", "rag-hero", "2.0.0-beta.1"),
    ];
    vi.mocked(global.fetch).mockResolvedValue({
      ok: true,
      json: async () => packs,
    } as Response);

    const svc = new WorkspaceApiService();
    const result = await svc.getPromptPack("team acme", "rag-hero");

    expect(global.fetch).toHaveBeenCalledWith(
      "/api/workspaces/team%20acme/promptpacks?labelSelector=omnia.altairalabs.ai%2Fpromptpack%3Drag-hero"
    );
    // Channel-max stable => 1.1.0 (not the 2.0.0 prerelease).
    expect(result?.metadata.name).toBe("pp-v110");
    expect(result?.spec.version).toBe("1.1.0");
  });

  it("returns undefined when the label selector matches nothing", async () => {
    vi.mocked(global.fetch).mockResolvedValue({
      ok: true,
      json: async () => [],
    } as Response);

    const svc = new WorkspaceApiService();
    expect(await svc.getPromptPack("w", "missing")).toBeUndefined();
  });

  it("returns undefined on a 404 response", async () => {
    vi.mocked(global.fetch).mockResolvedValue({ ok: false, status: 404 } as Response);
    const svc = new WorkspaceApiService();
    expect(await svc.getPromptPack("w", "gone")).toBeUndefined();
  });

  it("throws on a non-404 error response", async () => {
    vi.mocked(global.fetch).mockResolvedValue({
      ok: false,
      status: 500,
      statusText: "Server Error",
    } as Response);
    const svc = new WorkspaceApiService();
    await expect(svc.getPromptPack("w", "x")).rejects.toThrow("Server Error");
  });
});
