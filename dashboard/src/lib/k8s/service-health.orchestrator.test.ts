import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("./workspace-route-helpers", () => ({
  getWorkspace: vi.fn(),
}));

vi.mock("./workspace-k8s-client-factory", () => ({
  getWorkspaceCoreApi: vi.fn(),
}));

import { getServiceHealth } from "./service-health";
import { getWorkspace } from "./workspace-route-helpers";
import { getWorkspaceCoreApi } from "./workspace-k8s-client-factory";

const clientOptions = { workspace: "acme", namespace: "omnia-acme", role: "viewer" as const };

const readyPod = {
  status: {
    phase: "Running",
    containerStatuses: [{ ready: true, restartCount: 0, state: { running: {} } }],
  },
} as any;

const crashPod = {
  status: {
    phase: "Running",
    containerStatuses: [
      {
        ready: false,
        restartCount: 7,
        state: { waiting: { reason: "CrashLoopBackOff", message: "back-off restarting" } },
        lastState: { terminated: { reason: "Error", message: "boom" } },
      },
    ],
  },
} as any;

function workspaceFixture() {
  return {
    spec: { namespace: { name: "omnia-acme" } },
    status: {
      services: [
        { name: "default", sessionURL: "https://session-default:8080", memoryURL: "https://memory-default:8080", ready: true },
        { name: "beta", sessionURL: "https://session-beta:8080", memoryURL: "https://memory-beta:8080", ready: false },
      ],
      privacyURL: "https://privacy-acme:8080",
    },
  } as any;
}

function makeCoreApiMock(podsBySelector: Record<string, any[]>) {
  return {
    listNamespacedPod: vi.fn(async ({ labelSelector }: { labelSelector: string }) => ({
      items: podsBySelector[labelSelector] ?? [],
    })),
  };
}

describe("getServiceHealth", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("assembles group + workspace-level health from CRD status, keyed by label selector", async () => {
    vi.mocked(getWorkspace).mockResolvedValue(workspaceFixture());

    const podsBySelector: Record<string, any[]> = {
      "app.kubernetes.io/component=session-api,omnia.altairalabs.ai/service-group=default": [readyPod],
      "app.kubernetes.io/component=memory-api,omnia.altairalabs.ai/service-group=default": [readyPod],
      "app.kubernetes.io/component=session-api,omnia.altairalabs.ai/service-group=beta": [readyPod],
      "app.kubernetes.io/component=memory-api,omnia.altairalabs.ai/service-group=beta": [crashPod],
      "app.kubernetes.io/component=privacy-api": [readyPod],
    };
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue(makeCoreApiMock(podsBySelector) as any);

    const result = await getServiceHealth(clientOptions, "acme");

    expect(result.source).toBe("crd");
    expect(result.groups).toHaveLength(2);

    const defaultGroup = result.groups.find((g) => g.name === "default")!;
    expect(defaultGroup.ready).toBe(true);
    expect(defaultGroup.members).toEqual([
      expect.objectContaining({ service: "session-api", state: "ready", url: "https://session-default:8080" }),
      expect.objectContaining({ service: "memory-api", state: "ready", url: "https://memory-default:8080" }),
    ]);

    const betaGroup = result.groups.find((g) => g.name === "beta")!;
    expect(betaGroup.ready).toBe(false);
    const memoryMember = betaGroup.members.find((m) => m.service === "memory-api")!;
    expect(memoryMember.state).toBe("crashlooping");
    expect(memoryMember.restarts).toBe(7);

    expect(result.workspaceServices).toEqual([
      expect.objectContaining({ service: "privacy-api", state: "ready", url: "https://privacy-acme:8080" }),
    ]);
  });

  it("falls back to unknown state when the k8s calls throw", async () => {
    vi.mocked(getWorkspace).mockResolvedValue(workspaceFixture());
    vi.mocked(getWorkspaceCoreApi).mockRejectedValue(new Error("connection refused"));

    const result = await getServiceHealth(clientOptions, "acme");

    expect(result.source).toBe("unknown");
    for (const group of result.groups) {
      for (const member of group.members) {
        expect(member.state).toBe("unknown");
      }
    }
    for (const service of result.workspaceServices) {
      expect(service.state).toBe("unknown");
    }
  });

  it("uses env fallback URLs when the workspace has no CRD service status", async () => {
    vi.mocked(getWorkspace).mockResolvedValue({
      spec: { namespace: { name: "omnia-acme" } },
      status: {},
    } as any);

    const podsBySelector: Record<string, any[]> = {
      "app.kubernetes.io/component=session-api,omnia.altairalabs.ai/service-group=default": [readyPod],
      "app.kubernetes.io/component=memory-api,omnia.altairalabs.ai/service-group=default": [readyPod],
      "app.kubernetes.io/component=privacy-api": [],
    };
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue(makeCoreApiMock(podsBySelector) as any);

    const originalSession = process.env.SESSION_API_URL;
    const originalMemory = process.env.MEMORY_API_URL;
    process.env.SESSION_API_URL = "https://session-env:8080";
    process.env.MEMORY_API_URL = "https://memory-env:8080";

    try {
      const result = await getServiceHealth(clientOptions, "acme");
      expect(result.source).toBe("envFallback");
      expect(result.groups).toHaveLength(1);
      expect(result.groups[0].members).toEqual([
        expect.objectContaining({ service: "session-api", state: "ready", url: "https://session-env:8080" }),
        expect.objectContaining({ service: "memory-api", state: "ready", url: "https://memory-env:8080" }),
      ]);
    } finally {
      process.env.SESSION_API_URL = originalSession;
      process.env.MEMORY_API_URL = originalMemory;
    }
  });
});
