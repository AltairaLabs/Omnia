import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Store mock functions
const mockListNamespacedSecret = vi.fn();
const mockListSecretForAllNamespaces = vi.fn();
const mockReadNamespacedSecret = vi.fn();
const mockCreateNamespacedSecret = vi.fn();
const mockReplaceNamespacedSecret = vi.fn();
const mockDeleteNamespacedSecret = vi.fn();
const mockListNamespace = vi.fn();
const mockListClusterCustomObject = vi.fn();

// Mock the kubernetes client-node module before importing the module under test
vi.mock("@kubernetes/client-node", () => {
  class MockKubeConfig {
    loadFromCluster() {
      throw new Error("Not in cluster");
    }
    loadFromDefault() {
      // no-op
    }
    makeApiClient() {
      return {
        listNamespacedSecret: mockListNamespacedSecret,
        listSecretForAllNamespaces: mockListSecretForAllNamespaces,
        readNamespacedSecret: mockReadNamespacedSecret,
        createNamespacedSecret: mockCreateNamespacedSecret,
        replaceNamespacedSecret: mockReplaceNamespacedSecret,
        deleteNamespacedSecret: mockDeleteNamespacedSecret,
        listNamespace: mockListNamespace,
        listClusterCustomObject: mockListClusterCustomObject,
      };
    }
  }

  return {
    KubeConfig: MockKubeConfig,
    CoreV1Api: vi.fn(),
    CustomObjectsApi: vi.fn(),
  };
});

// Import after mocking - use dynamic import to reset module state
let secretsModule: typeof import("./secrets");

describe("secrets", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // Reset module to clear singleton clients
    vi.resetModules();
    secretsModule = await import("./secrets");
    // Default mock for providers
    mockListClusterCustomObject.mockResolvedValue({ items: [] });
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  // Helper to create mock V1Secret
  function createMockSecret(
    namespace: string,
    name: string,
    keys: string[],
    hasLabel = true
  ) {
    const data: Record<string, string> = {};
    keys.forEach((key) => {
      data[key] = Buffer.from("mock-value").toString("base64");
    });

    return {
      metadata: {
        namespace,
        name,
        labels: hasLabel
          ? { [secretsModule.CREDENTIALS_LABEL]: secretsModule.CREDENTIALS_LABEL_VALUE }
          : {},
        annotations: { [secretsModule.PROVIDER_ANNOTATION]: "claude" },
        creationTimestamp: new Date("2024-01-15T10:00:00Z"),
        managedFields: [{ time: new Date("2024-01-15T12:00:00Z") }],
        resourceVersion: undefined as string | undefined,
      },
      data,
      type: "Opaque",
    };
  }

  describe("listSecrets", () => {
    it("should list secrets in a specific namespace", async () => {
      const mockSecret = createMockSecret("default", "test-secret", ["API_KEY"]);
      mockListNamespacedSecret.mockResolvedValue({ items: [mockSecret] });

      const result = await secretsModule.listSecrets("default");

      expect(mockListNamespacedSecret).toHaveBeenCalledWith({
        namespace: "default",
        labelSelector: `${secretsModule.CREDENTIALS_LABEL}=${secretsModule.CREDENTIALS_LABEL_VALUE}`,
      });
      expect(result).toHaveLength(1);
      expect(result[0].namespace).toBe("default");
      expect(result[0].name).toBe("test-secret");
      expect(result[0].keys).toEqual(["API_KEY"]);
    });

    it("should list secrets across all namespaces", async () => {
      const mockSecret1 = createMockSecret("default", "secret-1", ["KEY1"]);
      const mockSecret2 = createMockSecret("production", "secret-2", ["KEY2"]);
      mockListSecretForAllNamespaces.mockResolvedValue({
        items: [mockSecret1, mockSecret2],
      });

      const result = await secretsModule.listSecrets();

      expect(mockListSecretForAllNamespaces).toHaveBeenCalledWith({
        labelSelector: `${secretsModule.CREDENTIALS_LABEL}=${secretsModule.CREDENTIALS_LABEL_VALUE}`,
      });
      expect(result).toHaveLength(2);
    });

    it("should include provider references", async () => {
      const mockSecret = createMockSecret("default", "anthropic-creds", [
        "ANTHROPIC_API_KEY",
      ]);
      mockListNamespacedSecret.mockResolvedValue({ items: [mockSecret] });

      mockListClusterCustomObject.mockResolvedValue({
        items: [
          {
            metadata: { namespace: "default", name: "claude-provider" },
            spec: { type: "claude", secretRef: { name: "anthropic-creds" } },
          },
        ],
      });

      const result = await secretsModule.listSecrets("default");

      expect(result[0].referencedBy).toHaveLength(1);
      expect(result[0].referencedBy[0].name).toBe("claude-provider");
    });

    it("should never return secret values", async () => {
      const mockSecret = createMockSecret("default", "test-secret", ["API_KEY"]);
      mockListNamespacedSecret.mockResolvedValue({ items: [mockSecret] });

      const result = await secretsModule.listSecrets("default");

      expect(result[0].keys).toEqual(["API_KEY"]);
      expect((result[0] as unknown as Record<string, unknown>).data).toBeUndefined();
    });
  });

  describe("getSecret", () => {
    it("should get a single secret metadata", async () => {
      const mockSecret = createMockSecret("default", "test-secret", ["API_KEY"]);
      mockReadNamespacedSecret.mockResolvedValue(mockSecret);

      const result = await secretsModule.getSecret("default", "test-secret");

      expect(mockReadNamespacedSecret).toHaveBeenCalledWith({
        namespace: "default",
        name: "test-secret",
      });
      expect(result).not.toBeNull();
      expect(result?.name).toBe("test-secret");
      expect(result?.keys).toEqual(["API_KEY"]);
    });

    it("should return null for secrets without credentials label", async () => {
      const mockSecret = createMockSecret("default", "test-secret", ["API_KEY"], false);
      mockReadNamespacedSecret.mockResolvedValue(mockSecret);

      const result = await secretsModule.getSecret("default", "test-secret");

      expect(result).toBeNull();
    });

    it("should return null for non-existent secrets", async () => {
      mockReadNamespacedSecret.mockRejectedValue({ statusCode: 404 });

      const result = await secretsModule.getSecret("default", "non-existent");

      expect(result).toBeNull();
    });

    it("should return null for 404 error with nested response object", async () => {
      mockReadNamespacedSecret.mockRejectedValue({ response: { statusCode: 404 } });

      const result = await secretsModule.getSecret("default", "non-existent");

      expect(result).toBeNull();
    });

    it("should use creationTimestamp when no managedFields time exists", async () => {
      const mockSecret = {
        metadata: {
          namespace: "default",
          name: "test-secret",
          labels: { [secretsModule.CREDENTIALS_LABEL]: secretsModule.CREDENTIALS_LABEL_VALUE },
          annotations: { [secretsModule.PROVIDER_ANNOTATION]: "claude" },
          creationTimestamp: new Date("2024-01-15T10:00:00Z"),
          managedFields: [], // Empty managedFields
        },
        data: { API_KEY: Buffer.from("mock-value").toString("base64") },
        type: "Opaque",
      };
      mockReadNamespacedSecret.mockResolvedValue(mockSecret);

      const result = await secretsModule.getSecret("default", "test-secret");

      expect(result).not.toBeNull();
      expect(result?.modifiedAt).toBe("2024-01-15T10:00:00.000Z");
    });

    it("should handle secret with no creationTimestamp", async () => {
      const mockSecret = {
        metadata: {
          namespace: "default",
          name: "test-secret",
          labels: { [secretsModule.CREDENTIALS_LABEL]: secretsModule.CREDENTIALS_LABEL_VALUE },
          annotations: { [secretsModule.PROVIDER_ANNOTATION]: "claude" },
          managedFields: [], // Empty managedFields
          // No creationTimestamp
        },
        data: { API_KEY: Buffer.from("mock-value").toString("base64") },
        type: "Opaque",
      };
      mockReadNamespacedSecret.mockResolvedValue(mockSecret);

      const result = await secretsModule.getSecret("default", "test-secret");

      expect(result).not.toBeNull();
      expect(result?.modifiedAt).toBe("");
    });
  });

  describe("createOrUpdateSecret", () => {
    it("should create a new secret with credentials label", async () => {
      mockReadNamespacedSecret.mockRejectedValue({ statusCode: 404 });
      const createdSecret = createMockSecret("default", "new-secret", ["API_KEY"]);
      mockCreateNamespacedSecret.mockResolvedValue(createdSecret);
      // After create, getSecret is called which needs to return the created secret
      mockReadNamespacedSecret
        .mockRejectedValueOnce({ statusCode: 404 }) // First call - check if exists
        .mockResolvedValueOnce(createdSecret); // Second call - after create

      const result = await secretsModule.createOrUpdateSecret({
        namespace: "default",
        name: "new-secret",
        data: { API_KEY: "test-value" },
        providerType: "claude",
      });

      expect(mockCreateNamespacedSecret).toHaveBeenCalled();
      const createCall = mockCreateNamespacedSecret.mock.calls[0][0];
      expect(
        createCall.body.metadata.labels[secretsModule.CREDENTIALS_LABEL]
      ).toBe(secretsModule.CREDENTIALS_LABEL_VALUE);
      expect(result.name).toBe("new-secret");
    });

    it("should update an existing secret", async () => {
      const existingSecret = createMockSecret("default", "existing-secret", [
        "OLD_KEY",
      ]);
      existingSecret.metadata!.resourceVersion = "12345";

      const updatedSecret = createMockSecret("default", "existing-secret", [
        "OLD_KEY",
        "NEW_KEY",
      ]);

      // First read returns existing, replace succeeds, second read returns updated
      mockReadNamespacedSecret
        .mockResolvedValueOnce(existingSecret)
        .mockResolvedValueOnce(updatedSecret);
      mockReplaceNamespacedSecret.mockResolvedValue(updatedSecret);

      const result = await secretsModule.createOrUpdateSecret({
        namespace: "default",
        name: "existing-secret",
        data: { NEW_KEY: "new-value" },
      });

      expect(mockReplaceNamespacedSecret).toHaveBeenCalled();
      expect(result.keys).toContain("OLD_KEY");
      expect(result.keys).toContain("NEW_KEY");
    });

    it("should reject updating non-managed secrets", async () => {
      const existingSecret = createMockSecret("default", "unmanaged", ["KEY"], false);
      mockReadNamespacedSecret.mockResolvedValue(existingSecret);

      await expect(
        secretsModule.createOrUpdateSecret({
          namespace: "default",
          name: "unmanaged",
          data: { KEY: "value" },
        })
      ).rejects.toThrow("not a managed credential secret");
    });
  });

  describe("deleteSecret", () => {
    it("should delete a managed secret", async () => {
      const mockSecret = createMockSecret("default", "to-delete", ["KEY"]);
      mockReadNamespacedSecret.mockResolvedValue(mockSecret);
      mockDeleteNamespacedSecret.mockResolvedValue({});

      const result = await secretsModule.deleteSecret("default", "to-delete");

      expect(mockDeleteNamespacedSecret).toHaveBeenCalledWith({
        namespace: "default",
        name: "to-delete",
      });
      expect(result).toBe(true);
    });

    it("should return false for non-existent secrets", async () => {
      mockReadNamespacedSecret.mockRejectedValue({ statusCode: 404 });

      const result = await secretsModule.deleteSecret("default", "non-existent");

      expect(result).toBe(false);
    });

    it("should reject deleting non-managed secrets", async () => {
      const existingSecret = createMockSecret("default", "unmanaged", ["KEY"], false);
      mockReadNamespacedSecret.mockResolvedValue(existingSecret);

      await expect(
        secretsModule.deleteSecret("default", "unmanaged")
      ).rejects.toThrow("not a managed credential secret");
    });
  });

  describe("listNamespaces", () => {
    it("should list all namespaces", async () => {
      mockListNamespace.mockResolvedValue({
        items: [
          { metadata: { name: "default" } },
          { metadata: { name: "production" } },
          { metadata: { name: "staging" } },
        ],
      });

      const result = await secretsModule.listNamespaces();

      expect(result).toEqual(["default", "production", "staging"]);
    });

    it("should return sorted namespaces", async () => {
      mockListNamespace.mockResolvedValue({
        items: [
          { metadata: { name: "zebra" } },
          { metadata: { name: "alpha" } },
          { metadata: { name: "beta" } },
        ],
      });

      const result = await secretsModule.listNamespaces();

      expect(result).toEqual(["alpha", "beta", "zebra"]);
    });
  });

  describe("constants", () => {
    it("should export correct label constants", () => {
      expect(secretsModule.CREDENTIALS_LABEL).toBe("omnia.altairalabs.ai/type");
      expect(secretsModule.CREDENTIALS_LABEL_VALUE).toBe("credentials");
      expect(secretsModule.PROVIDER_ANNOTATION).toBe("omnia.altairalabs.ai/provider");
    });
  });
});
