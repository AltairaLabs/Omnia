import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Store mock functions
const mockCreateNamespacedServiceAccountToken = vi.fn();

// Mock the kubernetes client-node module
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
        createNamespacedServiceAccountToken: mockCreateNamespacedServiceAccountToken,
      };
    }
  }

  return {
    KubeConfig: MockKubeConfig,
    CoreV1Api: vi.fn(),
  };
});

// Mock the token-cache module
vi.mock("./token-cache", () => ({
  getCachedToken: vi.fn(),
  setCachedToken: vi.fn(),
  invalidateToken: vi.fn(),
}));

// Import mocked modules
import { getCachedToken, setCachedToken, invalidateToken } from "./token-cache";

// Import after mocking - use dynamic import to reset module state
let tokenFetcherModule: typeof import("./token-fetcher");

describe("token-fetcher", () => {
  const originalEnv = process.env;

  beforeEach(async () => {
    vi.clearAllMocks();
    vi.resetModules();

    // Reset environment
    process.env = { ...originalEnv };
    delete process.env.OMNIA_K8S_DEV_MODE;
    delete process.env.OMNIA_K8S_DEV_TOKEN;

    tokenFetcherModule = await import("./token-fetcher");
    tokenFetcherModule.resetKubeConfig();
  });

  afterEach(() => {
    process.env = originalEnv;
    vi.clearAllMocks();
  });

  describe("getServiceAccountName", () => {
    it("should generate correct SA name for owner", () => {
      const name = tokenFetcherModule.getServiceAccountName("my-workspace", "owner");
      expect(name).toBe("workspace-my-workspace-owner-sa");
    });

    it("should generate correct SA name for editor", () => {
      const name = tokenFetcherModule.getServiceAccountName("my-workspace", "editor");
      expect(name).toBe("workspace-my-workspace-editor-sa");
    });

    it("should generate correct SA name for viewer", () => {
      const name = tokenFetcherModule.getServiceAccountName("my-workspace", "viewer");
      expect(name).toBe("workspace-my-workspace-viewer-sa");
    });
  });

  describe("fetchServiceAccountToken", () => {
    it("should fetch token from K8s API", async () => {
      const mockToken = "mock-jwt-token";
      const mockExpiration = new Date(Date.now() + 3600000).toISOString();

      mockCreateNamespacedServiceAccountToken.mockResolvedValue({
        status: {
          token: mockToken,
          expirationTimestamp: mockExpiration,
        },
      });

      const result = await tokenFetcherModule.fetchServiceAccountToken(
        "my-workspace",
        "my-namespace",
        "editor"
      );

      expect(mockCreateNamespacedServiceAccountToken).toHaveBeenCalledWith({
        name: "workspace-my-workspace-editor-sa",
        namespace: "my-namespace",
        body: {
          apiVersion: "authentication.k8s.io/v1",
          kind: "TokenRequest",
          spec: {
            audiences: [],
            expirationSeconds: 3600,
          },
        },
      });

      expect(result.token).toBe(mockToken);
      expect(result.expiresAt).toBe(new Date(mockExpiration).getTime());
    });

    it("should throw error when token not in response", async () => {
      mockCreateNamespacedServiceAccountToken.mockResolvedValue({
        status: {},
      });

      await expect(
        tokenFetcherModule.fetchServiceAccountToken(
          "my-workspace",
          "my-namespace",
          "editor"
        )
      ).rejects.toThrow("TokenRequest response did not contain a token");
    });

    it("should wrap K8s API errors with context", async () => {
      mockCreateNamespacedServiceAccountToken.mockRejectedValue(
        new Error("Forbidden")
      );

      await expect(
        tokenFetcherModule.fetchServiceAccountToken(
          "my-workspace",
          "my-namespace",
          "editor"
        )
      ).rejects.toThrow(
        "Failed to fetch token for SA workspace-my-workspace-editor-sa in namespace my-namespace: Forbidden"
      );
    });

    it("should use dev token in dev mode", async () => {
      process.env.OMNIA_K8S_DEV_MODE = "true";
      process.env.OMNIA_K8S_DEV_TOKEN = "dev-token-123";

      // Re-import to pick up env changes
      vi.resetModules();
      tokenFetcherModule = await import("./token-fetcher");

      const result = await tokenFetcherModule.fetchServiceAccountToken(
        "my-workspace",
        "my-namespace",
        "editor"
      );

      expect(result.token).toBe("dev-token-123");
      expect(mockCreateNamespacedServiceAccountToken).not.toHaveBeenCalled();
    });

    it("should throw error in dev mode when token not set", async () => {
      process.env.OMNIA_K8S_DEV_MODE = "true";
      delete process.env.OMNIA_K8S_DEV_TOKEN;

      // Re-import to pick up env changes
      vi.resetModules();
      tokenFetcherModule = await import("./token-fetcher");

      await expect(
        tokenFetcherModule.fetchServiceAccountToken(
          "my-workspace",
          "my-namespace",
          "editor"
        )
      ).rejects.toThrow(
        "OMNIA_K8S_DEV_MODE is enabled but OMNIA_K8S_DEV_TOKEN is not set"
      );
    });
  });

  describe("getWorkspaceToken", () => {
    it("should return cached token if available", async () => {
      vi.mocked(getCachedToken).mockReturnValue("cached-token");

      const result = await tokenFetcherModule.getWorkspaceToken(
        "my-workspace",
        "my-namespace",
        "editor"
      );

      expect(result).toBe("cached-token");
      expect(mockCreateNamespacedServiceAccountToken).not.toHaveBeenCalled();
    });

    it("should fetch and cache token if not cached", async () => {
      vi.mocked(getCachedToken).mockReturnValue(null);

      const mockToken = "new-token";
      const mockExpiration = new Date(Date.now() + 3600000).toISOString();

      mockCreateNamespacedServiceAccountToken.mockResolvedValue({
        status: {
          token: mockToken,
          expirationTimestamp: mockExpiration,
        },
      });

      const result = await tokenFetcherModule.getWorkspaceToken(
        "my-workspace",
        "my-namespace",
        "editor"
      );

      expect(result).toBe(mockToken);
      expect(setCachedToken).toHaveBeenCalledWith(
        "my-workspace",
        "editor",
        mockToken,
        expect.any(Number)
      );
    });
  });

  describe("refreshWorkspaceToken", () => {
    it("should invalidate cache and fetch new token", async () => {
      const mockToken = "refreshed-token";
      const mockExpiration = new Date(Date.now() + 3600000).toISOString();

      mockCreateNamespacedServiceAccountToken.mockResolvedValue({
        status: {
          token: mockToken,
          expirationTimestamp: mockExpiration,
        },
      });

      const result = await tokenFetcherModule.refreshWorkspaceToken(
        "my-workspace",
        "my-namespace",
        "editor"
      );

      expect(invalidateToken).toHaveBeenCalledWith("my-workspace", "editor");
      expect(result).toBe(mockToken);
      expect(setCachedToken).toHaveBeenCalledWith(
        "my-workspace",
        "editor",
        mockToken,
        expect.any(Number)
      );
    });
  });
});
