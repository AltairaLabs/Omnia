import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { gzipSync } from "node:zlib";
import * as tar from "tar-stream";

// Helper to create a valid tar.gz buffer for testing
async function createTarGzBuffer(files: Record<string, string>): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const pack = tar.pack();
    const chunks: Buffer[] = [];

    pack.on("data", (chunk: Buffer) => chunks.push(chunk));
    pack.on("end", () => {
      const tarBuffer = Buffer.concat(chunks);
      const gzipped = gzipSync(tarBuffer);
      resolve(gzipped);
    });
    pack.on("error", reject);

    for (const [name, content] of Object.entries(files)) {
      pack.entry({ name }, content);
    }
    pack.finalize();
  });
}

// Mock the token-fetcher module
vi.mock("./token-fetcher", () => ({
  getWorkspaceToken: vi.fn().mockResolvedValue("test-token"),
  refreshWorkspaceToken: vi.fn().mockResolvedValue("new-token"),
}));

// Mock API results
const mockAgentList = {
  items: [
    {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "AgentRuntime",
      metadata: { name: "agent-1", namespace: "workspace-ns" },
      spec: { replicas: 1 },
    },
    {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "AgentRuntime",
      metadata: { name: "agent-2", namespace: "workspace-ns" },
      spec: { replicas: 2 },
    },
  ],
};

const mockAgent = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "AgentRuntime",
  metadata: { name: "agent-1", namespace: "workspace-ns", resourceVersion: "12345" },
  spec: { replicas: 1 },
};

const mockPodList = {
  items: [
    {
      metadata: { name: "agent-1-pod-abc123" },
      spec: {
        containers: [{ name: "agent" }],
      },
    },
  ],
};

const mockEventList = {
  items: [
    {
      type: "Normal",
      reason: "Created",
      message: "Created container agent",
      firstTimestamp: new Date("2024-01-01T10:00:00Z"),
      lastTimestamp: new Date("2024-01-01T10:00:00Z"),
      count: 1,
      source: { component: "kubelet", host: "node-1" },
      involvedObject: { kind: "AgentRuntime", name: "agent-1", namespace: "workspace-ns" },
    },
  ],
};

// Mock API classes
const mockListNamespacedCustomObject = vi.fn();
const mockGetNamespacedCustomObject = vi.fn();
const mockCreateNamespacedCustomObject = vi.fn();
const mockReplaceNamespacedCustomObject = vi.fn();
const mockPatchNamespacedCustomObject = vi.fn();
const mockDeleteNamespacedCustomObject = vi.fn();
const mockListNamespacedPod = vi.fn();
const mockReadNamespacedPodLog = vi.fn();
const mockListNamespacedEvent = vi.fn();
const mockReadNamespacedConfigMap = vi.fn();
const mockPatchNamespacedDeploymentScale = vi.fn();

class MockCustomObjectsApi {
  listNamespacedCustomObject = mockListNamespacedCustomObject;
  getNamespacedCustomObject = mockGetNamespacedCustomObject;
  createNamespacedCustomObject = mockCreateNamespacedCustomObject;
  replaceNamespacedCustomObject = mockReplaceNamespacedCustomObject;
  patchNamespacedCustomObject = mockPatchNamespacedCustomObject;
  deleteNamespacedCustomObject = mockDeleteNamespacedCustomObject;
}

class MockCoreV1Api {
  listNamespacedPod = mockListNamespacedPod;
  readNamespacedPodLog = mockReadNamespacedPodLog;
  listNamespacedEvent = mockListNamespacedEvent;
  readNamespacedConfigMap = mockReadNamespacedConfigMap;
}

class MockAppsV1Api {
  patchNamespacedDeploymentScale = mockPatchNamespacedDeploymentScale;
}

// Mock the kubernetes client-node module
vi.mock("@kubernetes/client-node", () => {
  class MockKubeConfig {
    private clusters: Array<{ name: string; server: string; caData?: string }> = [];
    private currentCluster: { name: string; server: string; caData?: string } | null = null;

    loadFromCluster() {
      throw new Error("Not in cluster");
    }
    loadFromDefault() {
      this.clusters = [
        {
          name: "default-cluster",
          server: "https://kubernetes.default.svc",
          caData: "base64-ca-data",
        },
      ];
      this.currentCluster = this.clusters[0];
    }
    loadFromOptions(_options: unknown) {
      // Config loaded
    }
    loadFromClusterAndUser(cluster: { name: string; server: string; caData?: string }, _user: { name: string; token?: string }) {
      this.clusters = [cluster];
      this.currentCluster = cluster;
    }
    getCurrentCluster() {
      return this.currentCluster;
    }
    makeApiClient(ApiClass: new () => object) {
      return new ApiClass();
    }
  }

  return {
    KubeConfig: MockKubeConfig,
    CoreV1Api: MockCoreV1Api,
    CustomObjectsApi: MockCustomObjectsApi,
    AppsV1Api: MockAppsV1Api,
  };
});

// Import after mocking
let crdOperations: typeof import("./crd-operations");

describe("crd-operations", () => {
  const defaultOptions = {
    workspace: "my-workspace",
    namespace: "workspace-ns",
    role: "editor" as const,
  };

  beforeEach(async () => {
    vi.clearAllMocks();
    vi.resetModules();
    crdOperations = await import("./crd-operations");
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe("listCrd", () => {
    it("should list CRD resources in a namespace", async () => {
      mockListNamespacedCustomObject.mockResolvedValue(mockAgentList);

      const result = await crdOperations.listCrd<typeof mockAgent>(defaultOptions, "agentruntimes");

      expect(result).toHaveLength(2);
      expect(result[0].metadata.name).toBe("agent-1");
      expect(mockListNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "workspace-ns",
        plural: "agentruntimes",
      });
    });

    it("should return empty array when no items", async () => {
      mockListNamespacedCustomObject.mockResolvedValue({ items: [] });

      const result = await crdOperations.listCrd(defaultOptions, "agentruntimes");

      expect(result).toHaveLength(0);
    });

    it("should handle missing items field", async () => {
      mockListNamespacedCustomObject.mockResolvedValue({});

      const result = await crdOperations.listCrd(defaultOptions, "agentruntimes");

      expect(result).toHaveLength(0);
    });
  });

  describe("getCrd", () => {
    it("should get a single CRD resource by name", async () => {
      mockGetNamespacedCustomObject.mockResolvedValue(mockAgent);

      const result = await crdOperations.getCrd<typeof mockAgent>(defaultOptions, "agentruntimes", "agent-1");

      expect(result).toBeDefined();
      expect(result?.metadata.name).toBe("agent-1");
      expect(mockGetNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "workspace-ns",
        plural: "agentruntimes",
        name: "agent-1",
      });
    });

    it("should return null for not found errors", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ statusCode: 404 });

      const result = await crdOperations.getCrd(defaultOptions, "agentruntimes", "missing");

      expect(result).toBeNull();
    });

    it("should return null for response.statusCode 404", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ response: { statusCode: 404 } });

      const result = await crdOperations.getCrd(defaultOptions, "agentruntimes", "missing");

      expect(result).toBeNull();
    });

    it("should throw non-404 errors", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue(new Error("Server error"));

      await expect(
        crdOperations.getCrd(defaultOptions, "agentruntimes", "agent-1")
      ).rejects.toThrow("Server error");
    });
  });

  describe("createCrd", () => {
    it("should create a CRD resource", async () => {
      const newAgent = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "AgentRuntime",
        metadata: { name: "new-agent" },
        spec: { replicas: 1 },
      };
      const createdAgent = { ...newAgent, metadata: { ...newAgent.metadata, namespace: "workspace-ns" } };
      mockCreateNamespacedCustomObject.mockResolvedValue(createdAgent);

      const result = await crdOperations.createCrd<typeof newAgent>(defaultOptions, "agentruntimes", newAgent);

      expect(result.metadata.name).toBe("new-agent");
      expect(mockCreateNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "workspace-ns",
        plural: "agentruntimes",
        body: newAgent,
      });
    });
  });

  describe("updateCrd", () => {
    it("should update a CRD resource", async () => {
      const updatedAgent = { ...mockAgent, spec: { replicas: 3 } };
      mockReplaceNamespacedCustomObject.mockResolvedValue(updatedAgent);

      const result = await crdOperations.updateCrd(
        defaultOptions,
        "agentruntimes",
        "agent-1",
        updatedAgent
      );

      expect(result.spec.replicas).toBe(3);
      expect(mockReplaceNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "workspace-ns",
        plural: "agentruntimes",
        name: "agent-1",
        body: updatedAgent,
      });
    });
  });

  describe("patchCrd", () => {
    it("should patch a CRD resource", async () => {
      const patchedAgent = { ...mockAgent, spec: { replicas: 5 } };
      mockPatchNamespacedCustomObject.mockResolvedValue(patchedAgent);

      const result = await crdOperations.patchCrd<typeof mockAgent>(
        defaultOptions,
        "agentruntimes",
        "agent-1",
        { spec: { replicas: 5 } }
      );

      expect(result.spec.replicas).toBe(5);
      expect(mockPatchNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "workspace-ns",
        plural: "agentruntimes",
        name: "agent-1",
        body: { spec: { replicas: 5 } },
      });
    });
  });

  describe("deleteCrd", () => {
    it("should delete a CRD resource", async () => {
      mockDeleteNamespacedCustomObject.mockResolvedValue({});

      await crdOperations.deleteCrd(defaultOptions, "agentruntimes", "agent-1");

      expect(mockDeleteNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "workspace-ns",
        plural: "agentruntimes",
        name: "agent-1",
      });
    });
  });

  describe("getPodLogs", () => {
    it("should get logs from pods matching a label selector", async () => {
      mockListNamespacedPod.mockResolvedValue(mockPodList);
      mockReadNamespacedPodLog.mockResolvedValue(
        "2024-01-01T10:00:00.000Z Log message 1\n2024-01-01T10:00:01.000Z Log message 2"
      );

      const result = await crdOperations.getPodLogs(
        defaultOptions,
        "app.kubernetes.io/instance=agent-1",
        100
      );

      expect(result).toHaveLength(2);
      expect(result[0].message).toBe("Log message 1");
      expect(result[0].timestamp).toBe("2024-01-01T10:00:00.000Z");
      expect(mockListNamespacedPod).toHaveBeenCalledWith({
        namespace: "workspace-ns",
        labelSelector: "app.kubernetes.io/instance=agent-1",
      });
    });

    it("should return empty array when no pods found", async () => {
      mockListNamespacedPod.mockResolvedValue({ items: [] });

      const result = await crdOperations.getPodLogs(
        defaultOptions,
        "app.kubernetes.io/instance=missing"
      );

      expect(result).toHaveLength(0);
    });

    it("should handle log fetch errors gracefully", async () => {
      mockListNamespacedPod.mockResolvedValue(mockPodList);
      mockReadNamespacedPodLog.mockRejectedValue(new Error("Container not found"));

      const result = await crdOperations.getPodLogs(
        defaultOptions,
        "app.kubernetes.io/instance=agent-1"
      );

      expect(result).toHaveLength(0); // Should not throw, just return empty
    });

    it("should handle log lines without valid timestamps", async () => {
      mockListNamespacedPod.mockResolvedValue(mockPodList);
      // Log lines without ISO timestamp format
      mockReadNamespacedPodLog.mockResolvedValue(
        "Log message without timestamp\nAnother message"
      );

      const result = await crdOperations.getPodLogs(
        defaultOptions,
        "app.kubernetes.io/instance=agent-1",
        100
      );

      expect(result).toHaveLength(2);
      expect(result[0].message).toBe("Log message without timestamp");
      // Should have a timestamp (current time)
      expect(result[0].timestamp).toBeDefined();
    });

    it("should handle log lines with invalid timestamp prefix", async () => {
      mockListNamespacedPod.mockResolvedValue(mockPodList);
      // Log lines with space but not a valid timestamp
      mockReadNamespacedPodLog.mockResolvedValue(
        "INFO Log with level prefix\n01-01-2024 Wrong format"
      );

      const result = await crdOperations.getPodLogs(
        defaultOptions,
        "app.kubernetes.io/instance=agent-1",
        100
      );

      expect(result).toHaveLength(2);
      // These should fall back to using current timestamp
      expect(result[0].timestamp).toBeDefined();
      expect(result[1].timestamp).toBeDefined();
    });
  });

  describe("getResourceEvents", () => {
    it("should get events for a resource", async () => {
      mockListNamespacedEvent.mockResolvedValue(mockEventList);

      const result = await crdOperations.getResourceEvents(
        defaultOptions,
        "AgentRuntime",
        "agent-1"
      );

      expect(result).toHaveLength(1);
      expect(result[0].reason).toBe("Created");
      expect(result[0].type).toBe("Normal");
      expect(mockListNamespacedEvent).toHaveBeenCalledWith({
        namespace: "workspace-ns",
        fieldSelector: "involvedObject.kind=AgentRuntime,involvedObject.name=agent-1",
      });
    });

    it("should return empty array when no events found", async () => {
      mockListNamespacedEvent.mockResolvedValue({ items: [] });

      const result = await crdOperations.getResourceEvents(
        defaultOptions,
        "AgentRuntime",
        "agent-1"
      );

      expect(result).toHaveLength(0);
    });
  });

  describe("listSharedCrd", () => {
    it("should list shared CRDs using system-level access", async () => {
      mockListNamespacedCustomObject.mockResolvedValue({
        items: [
          { metadata: { name: "tool-registry-1" } },
          { metadata: { name: "tool-registry-2" } },
        ],
      });

      const result = await crdOperations.listSharedCrd("toolregistries", "omnia-system");

      expect(result).toHaveLength(2);
      expect(mockListNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "omnia-system",
        plural: "toolregistries",
      });
    });
  });

  describe("getSharedCrd", () => {
    it("should get a shared CRD by name", async () => {
      const mockToolRegistry = {
        metadata: { name: "tool-registry-1", namespace: "omnia-system" },
        spec: { url: "http://tools.example.com" },
      };
      mockGetNamespacedCustomObject.mockResolvedValue(mockToolRegistry);

      const result = await crdOperations.getSharedCrd<typeof mockToolRegistry>(
        "toolregistries",
        "omnia-system",
        "tool-registry-1"
      );

      expect(result?.metadata.name).toBe("tool-registry-1");
    });

    it("should return null for not found", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ statusCode: 404 });

      const result = await crdOperations.getSharedCrd(
        "toolregistries",
        "omnia-system",
        "missing"
      );

      expect(result).toBeNull();
    });

    it("should throw for non-404 errors", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue(new Error("Server error"));

      await expect(
        crdOperations.getSharedCrd("toolregistries", "omnia-system", "test")
      ).rejects.toThrow("Server error");
    });
  });

  describe("getConfigMapContent", () => {
    it("should get ConfigMap data", async () => {
      mockReadNamespacedConfigMap.mockResolvedValue({
        data: {
          "pack.yaml": "prompts:\n  main:\n    template: Hello",
        },
      });

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "my-prompt-pack-config"
      );

      expect(result).toBeDefined();
      expect(result?.["pack.yaml"]).toContain("prompts:");
    });

    it("should return null for not found", async () => {
      mockReadNamespacedConfigMap.mockRejectedValue({ statusCode: 404 });

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "missing-config"
      );

      expect(result).toBeNull();
    });

    it("should throw for non-404 errors", async () => {
      mockReadNamespacedConfigMap.mockRejectedValue(new Error("Server error"));

      await expect(
        crdOperations.getConfigMapContent(defaultOptions, "my-config")
      ).rejects.toThrow("Server error");
    });

    it("should return null for ConfigMap with no data", async () => {
      mockReadNamespacedConfigMap.mockResolvedValue({});

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "empty-config"
      );

      expect(result).toBeNull();
    });

    it("should extract files from tar.gz binaryData", async () => {
      const tarGzBuffer = await createTarGzBuffer({
        "config.yaml": "apiVersion: v1\nkind: Arena",
        "prompts/greeting.yaml": "kind: PromptConfig",
      });

      mockReadNamespacedConfigMap.mockResolvedValue({
        binaryData: {
          "pack.tar.gz": tarGzBuffer.toString("base64"),
        },
      });

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "my-pack-config"
      );

      expect(result).toBeDefined();
      expect(result?.["config.yaml"]).toContain("apiVersion: v1");
      expect(result?.["prompts/greeting.yaml"]).toContain("PromptConfig");
    });

    it("should fall back to data when binaryData has no tar.gz", async () => {
      mockReadNamespacedConfigMap.mockResolvedValue({
        binaryData: {
          "other.bin": Buffer.from("some binary").toString("base64"),
        },
        data: {
          "fallback.yaml": "content: fallback",
        },
      });

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "my-config"
      );

      expect(result).toBeDefined();
      expect(result?.["fallback.yaml"]).toBe("content: fallback");
    });

    it("should handle .tgz extension in binaryData", async () => {
      const tarGzBuffer = await createTarGzBuffer({
        "test.yaml": "kind: Test",
      });

      mockReadNamespacedConfigMap.mockResolvedValue({
        binaryData: {
          "pack.tgz": tarGzBuffer.toString("base64"),
        },
      });

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "my-tgz-config"
      );

      expect(result).toBeDefined();
      expect(result?.["test.yaml"]).toContain("kind: Test");
    });

    it("should fall back to data when tar.gz extraction yields no files", async () => {
      // Create an empty tar.gz (just end-of-archive markers)
      const emptyTarGz = gzipSync(Buffer.alloc(1024, 0));

      mockReadNamespacedConfigMap.mockResolvedValue({
        binaryData: {
          "pack.tar.gz": emptyTarGz.toString("base64"),
        },
        data: {
          "fallback.yaml": "fallback content",
        },
      });

      const result = await crdOperations.getConfigMapContent(
        defaultOptions,
        "my-empty-tar-config"
      );

      expect(result).toBeDefined();
      expect(result?.["fallback.yaml"]).toBe("fallback content");
    });
  });

  describe("scaleDeployment", () => {
    it("should scale a deployment to desired replicas", async () => {
      mockPatchNamespacedDeploymentScale.mockResolvedValue({});

      await crdOperations.scaleDeployment(defaultOptions, "my-deployment", 3);

      expect(mockPatchNamespacedDeploymentScale).toHaveBeenCalledWith({
        namespace: "workspace-ns",
        name: "my-deployment",
        body: {
          spec: {
            replicas: 3,
          },
        },
      });
    });

    it("should scale down to zero replicas", async () => {
      mockPatchNamespacedDeploymentScale.mockResolvedValue({});

      await crdOperations.scaleDeployment(defaultOptions, "my-deployment", 0);

      expect(mockPatchNamespacedDeploymentScale).toHaveBeenCalledWith({
        namespace: "workspace-ns",
        name: "my-deployment",
        body: {
          spec: {
            replicas: 0,
          },
        },
      });
    });
  });

  describe("error helpers", () => {
    describe("extractK8sErrorMessage", () => {
      it("should extract message from Error", () => {
        const error = new Error("Test error message");
        expect(crdOperations.extractK8sErrorMessage(error)).toBe("Test error message");
      });

      it("should extract message from body.message", () => {
        const error = { body: { message: "K8s API error" } };
        expect(crdOperations.extractK8sErrorMessage(error)).toBe("K8s API error");
      });

      it("should extract message from message property", () => {
        const error = { message: "Simple message" };
        expect(crdOperations.extractK8sErrorMessage(error)).toBe("Simple message");
      });

      it("should convert to string for unknown types", () => {
        expect(crdOperations.extractK8sErrorMessage("string error")).toBe("string error");
        expect(crdOperations.extractK8sErrorMessage(123)).toBe("123");
      });
    });

    describe("isForbiddenError", () => {
      it("should return true for statusCode 403", () => {
        expect(crdOperations.isForbiddenError({ statusCode: 403 })).toBe(true);
      });

      it("should return true for response.statusCode 403", () => {
        expect(crdOperations.isForbiddenError({ response: { statusCode: 403 } })).toBe(true);
      });

      it("should return false for other errors", () => {
        expect(crdOperations.isForbiddenError({ statusCode: 404 })).toBe(false);
        expect(crdOperations.isForbiddenError(new Error("test"))).toBe(false);
        expect(crdOperations.isForbiddenError(null)).toBe(false);
      });
    });
  });
});
