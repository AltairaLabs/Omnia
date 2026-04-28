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

      // After #1036: writes go to spec.credential.secretRef and the
      // legacy spec.secretRef is cleared in the same patch (the CRD
      // rejects "both set"). This regression-asserts both behaviours.
      expect(mockReplaceNamespacedCustomObject).toHaveBeenCalled();
      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.credential.secretRef.name).toBe("new-secret");
      expect(replaceCall.body.spec.secretRef).toBeUndefined();
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
      expect(replaceCall.body.spec.credential).toBeUndefined();
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
      expect(replaceCall.body.spec.credential.secretRef.name).toBe("new-secret");
      expect(replaceCall.body.spec.secretRef).toBeUndefined();
      expect(result).toEqual(updatedProvider);
    });

    it("removes the secretRef and drops the credential block when only credential.secretRef was set", async () => {
      // Provider already on the new shape with ONLY a secretRef in the
      // credential block: clearing it should also drop the empty
      // credential block so the Provider doesn't carry an inert {}.
      const existingProvider = {
        ...createMockProvider("default", "new-shape"),
        spec: {
          type: "claude",
          model: "claude-sonnet-4-20250514",
          credential: { secretRef: { name: "old-secret" } },
        },
      };
      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockResolvedValue(existingProvider);

      await providersModule.updateProviderSecretRef("default", "new-shape", null);

      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.secretRef).toBeUndefined();
      expect(replaceCall.body.spec.credential).toBeUndefined();
    });

    it("preserves other credential fields when clearing only secretRef", async () => {
      // If credential carries an envVar alongside secretRef, removing
      // the secretRef should NOT delete the rest of the credential block.
      const existingProvider = {
        ...createMockProvider("default", "mixed"),
        spec: {
          type: "claude",
          model: "claude-sonnet-4-20250514",
          credential: {
            secretRef: { name: "old-secret" },
            envVar: "MY_ENV",
          },
        },
      };
      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockResolvedValue(existingProvider);

      await providersModule.updateProviderSecretRef("default", "mixed", null);

      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.credential).toEqual({ envVar: "MY_ENV" });
    });

    it("should migrate legacy spec.secretRef to spec.credential.secretRef on next write", async () => {
      // A Provider that started life on the legacy field gets migrated
      // automatically when the dashboard updates its secretRef. Without
      // this the dashboard would write spec.credential while leaving
      // spec.secretRef in place, and the CRD's "exactly one" validation
      // would reject the patch.
      const existingProvider = createMockProvider("default", "legacy", "old-secret");
      mockGetNamespacedCustomObject.mockResolvedValue(existingProvider);
      mockReplaceNamespacedCustomObject.mockResolvedValue(existingProvider);

      await providersModule.updateProviderSecretRef("default", "legacy", "new-secret");

      const replaceCall = mockReplaceNamespacedCustomObject.mock.calls[0][0];
      expect(replaceCall.body.spec.credential.secretRef.name).toBe("new-secret");
      expect(replaceCall.body.spec.secretRef).toBeUndefined();
    });
  });

  describe("effectiveSecretRefName", () => {
    it("returns the new spec.credential.secretRef name", () => {
      expect(
        providersModule.effectiveSecretRefName({
          spec: { credential: { secretRef: { name: "new-name" } } },
        }),
      ).toBe("new-name");
    });

    it("falls back to legacy spec.secretRef.name", () => {
      expect(
        providersModule.effectiveSecretRefName({
          spec: { secretRef: { name: "legacy-name" } },
        }),
      ).toBe("legacy-name");
    });

    it("prefers credential.secretRef when both are set", () => {
      // Reflects operator's pkg/k8s/EffectiveSecretRef precedence —
      // dashboard must agree or it'll show one secret while the
      // runtime uses another. The CRD admission controller rejects
      // both-set, so this case shouldn't reach prod, but the helper
      // is the right place to define the resolution rule anyway.
      expect(
        providersModule.effectiveSecretRefName({
          spec: {
            credential: { secretRef: { name: "new" } },
            secretRef: { name: "old" },
          },
        }),
      ).toBe("new");
    });

    it("returns undefined when neither is set", () => {
      expect(providersModule.effectiveSecretRefName({ spec: {} })).toBeUndefined();
    });

    it("handles null/undefined provider", () => {
      expect(providersModule.effectiveSecretRefName(null)).toBeUndefined();
      expect(providersModule.effectiveSecretRefName(undefined)).toBeUndefined();
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
