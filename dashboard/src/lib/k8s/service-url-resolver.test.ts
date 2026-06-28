import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("./workspace-route-helpers", () => ({ getWorkspace: vi.fn() }));

const readyService = {
  name: "default",
  sessionURL: "https://session",
  memoryURL: "https://memory",
  ready: true,
};

const workspaceWithPrivacy = {
  spec: { namespace: { name: "omnia-default" } },
  status: { services: [readyService], privacyURL: "https://privacy-ws.ns:8080" },
};

describe("resolveServiceURLs", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("resolves the backing namespace from spec.namespace.name, NOT the workspace name (#1257)", async () => {
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      spec: { namespace: { name: "omnia-default" } },
      status: { services: [readyService] },
    } as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    const urls = await resolveServiceURLs("default");

    expect(urls?.namespace).toBe("omnia-default");
    expect(urls?.namespace).not.toBe("default");
    expect(urls?.sessionURL).toBe("https://session");
  });

  it("resolves privacyURL from workspace.status.privacyURL (CRD path)", async () => {
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(workspaceWithPrivacy as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    const urls = await resolveServiceURLs("default");

    expect(urls?.privacyURL).toBe("https://privacy-ws.ns:8080");
  });

  it("returns empty privacyURL when status.privacyURL is absent (CRD path)", async () => {
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      spec: { namespace: { name: "omnia-default" } },
      status: { services: [readyService] },
    } as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    const urls = await resolveServiceURLs("default");

    expect(urls?.privacyURL).toBe("");
  });

  it("falls back to status.namespace.name when spec namespace is absent", async () => {
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      status: { namespace: { name: "omnia-from-status" }, services: [readyService] },
    } as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    expect((await resolveServiceURLs("x"))?.namespace).toBe("omnia-from-status");
  });

  it("falls back to the workspace name when neither spec nor status namespace is set", async () => {
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      status: { services: [readyService] },
    } as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    expect((await resolveServiceURLs("ws"))?.namespace).toBe("ws");
  });

  it("uses the env fallback, defaulting namespace to the workspace name", async () => {
    vi.stubEnv("SESSION_API_URL", "https://env-session");
    vi.stubEnv("MEMORY_API_URL", "https://env-memory");
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    const urls = await resolveServiceURLs("ws");

    expect(urls?.sessionURL).toBe("https://env-session");
    expect(urls?.namespace).toBe("ws");
  });

  it("env fallback honours the SESSION_API_NAMESPACE override", async () => {
    vi.stubEnv("SESSION_API_URL", "https://env-session");
    vi.stubEnv("MEMORY_API_URL", "https://env-memory");
    vi.stubEnv("SESSION_API_NAMESPACE", "omnia-env");
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    expect((await resolveServiceURLs("ws"))?.namespace).toBe("omnia-env");
  });

  it("env fallback resolves privacyURL from PRIVACY_API_URL", async () => {
    vi.stubEnv("SESSION_API_URL", "https://env-session");
    vi.stubEnv("MEMORY_API_URL", "https://env-memory");
    vi.stubEnv("PRIVACY_API_URL", "https://env-privacy:8080");
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    const urls = await resolveServiceURLs("ws");

    expect(urls?.privacyURL).toBe("https://env-privacy:8080");
  });

  it("env fallback returns empty privacyURL when PRIVACY_API_URL is not set", async () => {
    vi.stubEnv("SESSION_API_URL", "https://env-session");
    vi.stubEnv("MEMORY_API_URL", "https://env-memory");
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    const urls = await resolveServiceURLs("ws");

    expect(urls?.privacyURL).toBe("");
  });

  it("returns null when there is no ready service and no env fallback", async () => {
    const { getWorkspace } = await import("./workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { resolveServiceURLs } = await import("./service-url-resolver");
    expect(await resolveServiceURLs("ws")).toBeNull();
  });
});
