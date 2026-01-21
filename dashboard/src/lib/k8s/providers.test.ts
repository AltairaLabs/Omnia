import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Store mock functions
const mockGetNamespacedCustomObject = vi.fn();
const mockReplaceNamespacedCustomObject = vi.fn();

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
        getNamespacedCustomObject: mockGetNamespacedCustomObject,
        replaceNamespacedCustomObject: mockReplaceNamespacedCustomObject,
      };
    }
  }

  return {
    KubeConfig: MockKubeConfig,
    CustomObjectsApi: vi.fn(),
  };
});

// Import after mocking - use dynamic import to reset module state
let providersModule: typeof import("./providers");

describe("providers", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // Reset module to clear singleton client
    vi.resetModules();
    providersModule = await import("./providers");
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  // Helper to create mock Provider resource
  function createMockProvider(
    namespace: string,
    name: string,
    secretRefName?: string
  ) {
    return {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        namespace,
        name,
        resourceVersion: "12345",
      },
      spec: {
        type: "claude",
        model: "claude-sonnet-4-20250514",
        ...(secretRefName && { secretRef: { name: secretRefName } }),
      },
      status: {
        phase: "Ready",
      },
    };
  }

  describe("getProvider", () => {
    it("should get a provider by namespace and name", async () => {
      const mockProvider = createMockProvider("default", "claude-provider", "anthropic-creds");
      mockGetNamespacedCustomObject.mockResolvedValue(mockProvider);

      const result = await providersModule.getProvider("default", "claude-provider");

      expect(mockGetNamespacedCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        namespace: "default",
        plural: "providers",
        name: "claude-provider",
      });
      expect(result).toEqual(mockProvider);
    });

    it("should return null for non-existent provider", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ statusCode: 404 });

      const result = await providersModule.getProvider("default", "non-existent");

      expect(result).toBeNull();
    });

    it("should return null for 404 with nested response object", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ response: { statusCode: 404 } });

      const result = await providersModule.getProvider("default", "non-existent");

      expect(result).toBeNull();
    });

    it("should throw for nested response with non-404 status", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ response: { statusCode: 500 } });

      await expect(providersModule.getProvider("default", "test")).rejects.toEqual({ response: { statusCode: 500 } });
    });

    it("should throw error for other API errors", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue(new Error("API error"));

      await expect(providersModule.getProvider("default", "test")).rejects.toThrow("API error");
    });
  });

  describe("updateProviderSecretRef", () => {
    it("should update secretRef to a new secret", async () => {
      const existingProvider = createMockProvider("default", "claude-provider", "old-secret");
      const updatedProvider = createMockProvider("default", "claude-provider", "new-secret");

      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockResolvedValue(updatedProvider);

      const result = await providersModule.updateProviderSecretRef(
        "default",
        "claude-provider",
        "new-secret"
      );

      expect(mockReplaceNamespacedCustomObject).toHaveBeenCalled();
      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.secretRef.name).toBe("new-secret");
      expect(result).toEqual(updatedProvider);
    });

    it("should remove secretRef when passed null", async () => {
      const existingProvider = createMockProvider("default", "claude-provider", "old-secret");
      const updatedProvider = createMockProvider("default", "claude-provider");

      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockResolvedValue(updatedProvider);

      const result = await providersModule.updateProviderSecretRef(
        "default",
        "claude-provider",
        null
      );

      expect(mockReplaceNamespacedCustomObject).toHaveBeenCalled();
      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.secretRef).toBeUndefined();
      expect(result).toEqual(updatedProvider);
    });

    it("should add secretRef to provider without one", async () => {
      const existingProvider = createMockProvider("default", "mock-provider");
      const updatedProvider = createMockProvider("default", "mock-provider", "new-secret");

      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockResolvedValue(updatedProvider);

      const result = await providersModule.updateProviderSecretRef(
        "default",
        "mock-provider",
        "new-secret"
      );

      expect(mockReplaceNamespacedCustomObject).toHaveBeenCalled();
      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.secretRef.name).toBe("new-secret");
      expect(result).toEqual(updatedProvider);
    });

    it("should throw error if provider not found", async () => {
      mockGetNamespacedCustomObject.mockRejectedValue({ statusCode: 404 });

      await expect(
        providersModule.updateProviderSecretRef("default", "non-existent", "secret")
      ).rejects.toThrow("Provider default/non-existent not found");
    });

    it("should throw error for API failures", async () => {
      const existingProvider = createMockProvider("default", "claude-provider");
      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockRejectedValue(new Error("Update failed"));

      await expect(
        providersModule.updateProviderSecretRef("default", "claude-provider", "secret")
      ).rejects.toThrow("Update failed");
    });
  });
});
