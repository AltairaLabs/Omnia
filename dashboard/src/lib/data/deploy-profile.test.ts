import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/k8s/crd-operations", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/crd-operations")>();
  return { ...actual, listCrd: vi.fn() };
});

const ready = { status: { phase: "Ready" } };
const clientOptions = { workspace: "team-acme", namespace: "ns", role: "viewer" as const };

describe("buildDeployProfile", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  it("maps Ready providers/skills and applies the default llm role", async () => {
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd)
      .mockResolvedValueOnce([
        { metadata: { name: "default" }, spec: { type: "claude", role: "llm", model: "m" }, ...ready },
        { metadata: { name: "legacy" }, spec: { type: "claude" }, ...ready },
        { metadata: { name: "down" }, spec: { type: "claude" }, status: { phase: "Error" } },
      ] as never)
      .mockResolvedValueOnce([{ metadata: { name: "docs" }, spec: { type: "git" }, ...ready }] as never);

    const { buildDeployProfile } = await import("./deploy-profile");
    const profile = await buildDeployProfile(clientOptions, "team-acme", "https://x");
    expect(profile).toEqual({
      api_endpoint: "https://x",
      workspace: "team-acme",
      providers: [
        { name: "default", role: "llm", type: "claude", model: "m" },
        { name: "legacy", role: "llm", type: "claude" },
      ],
      skills: [{ name: "docs", type: "git" }],
    });
  });
});
