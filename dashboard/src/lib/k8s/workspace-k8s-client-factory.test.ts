import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock the token-fetcher module
vi.mock("./token-fetcher", () => ({
  getWorkspaceToken: vi.fn(),
  refreshWorkspaceToken: vi.fn(),
}));

// Mock API classes
class MockCoreV1Api {
  listNamespacedPod = vi.fn();
}

class MockCustomObjectsApi {
  listNamespacedCustomObject = vi.fn();
}

class MockAppsV1Api {
  listNamespacedDeployment = vi.fn();
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

import { getWorkspaceToken, refreshWorkspaceToken } from "./token-fetcher";

// Import after mocking
let factoryModule: typeof import("./workspace-k8s-client-factory");

describe("workspace-k8s-client-factory", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    vi.resetModules();
    factoryModule = await import("./workspace-k8s-client-factory");
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe("getWorkspaceKubeConfig", () => {
    it("should create a KubeConfig with workspace token", async () => {
      vi.mocked(getWorkspaceToken).mockResolvedValue("test-token");

      const kc = await factoryModule.getWorkspaceKubeConfig({
        workspace: "my-workspace",
        namespace: "my-namespace",
        role: "editor",
      });

      expect(getWorkspaceToken).toHaveBeenCalledWith(
        "my-workspace",
        "my-namespace",
        "editor"
      );
      expect(kc).toBeDefined();
    });
  });

  describe("getWorkspaceCustomObjectsApi", () => {
    it("should create a CustomObjectsApi client", async () => {
      vi.mocked(getWorkspaceToken).mockResolvedValue("test-token");

      const api = await factoryModule.getWorkspaceCustomObjectsApi({
        workspace: "my-workspace",
        namespace: "my-namespace",
        role: "viewer",
      });

      expect(api).toBeDefined();
      expect(getWorkspaceToken).toHaveBeenCalledWith(
        "my-workspace",
        "my-namespace",
        "viewer"
      );
    });
  });

  describe("getWorkspaceCoreApi", () => {
    it("should create a CoreV1Api client", async () => {
      vi.mocked(getWorkspaceToken).mockResolvedValue("test-token");

      const api = await factoryModule.getWorkspaceCoreApi({
        workspace: "my-workspace",
        namespace: "my-namespace",
        role: "owner",
      });

      expect(api).toBeDefined();
      expect(getWorkspaceToken).toHaveBeenCalledWith(
        "my-workspace",
        "my-namespace",
        "owner"
      );
    });
  });

  describe("getWorkspaceAppsApi", () => {
    it("should create an AppsV1Api client", async () => {
      vi.mocked(getWorkspaceToken).mockResolvedValue("test-token");

      const api = await factoryModule.getWorkspaceAppsApi({
        workspace: "my-workspace",
        namespace: "my-namespace",
        role: "editor",
      });

      expect(api).toBeDefined();
      expect(getWorkspaceToken).toHaveBeenCalledWith(
        "my-workspace",
        "my-namespace",
        "editor"
      );
    });
  });

  describe("withTokenRefresh", () => {
    it("should return result on successful call", async () => {
      const mockFn = vi.fn().mockResolvedValue("success");

      const result = await factoryModule.withTokenRefresh(
        {
          workspace: "my-workspace",
          namespace: "my-namespace",
          role: "editor",
        },
        mockFn
      );

      expect(result).toBe("success");
      expect(mockFn).toHaveBeenCalledTimes(1);
      expect(refreshWorkspaceToken).not.toHaveBeenCalled();
    });

    it("should refresh token and retry on 401 error", async () => {
      vi.mocked(refreshWorkspaceToken).mockResolvedValue("new-token");

      const authError = { statusCode: 401 };
      const mockFn = vi
        .fn()
        .mockRejectedValueOnce(authError)
        .mockResolvedValueOnce("success after refresh");

      const result = await factoryModule.withTokenRefresh(
        {
          workspace: "my-workspace",
          namespace: "my-namespace",
          role: "editor",
        },
        mockFn
      );

      expect(result).toBe("success after refresh");
      expect(mockFn).toHaveBeenCalledTimes(2);
      expect(refreshWorkspaceToken).toHaveBeenCalledWith(
        "my-workspace",
        "my-namespace",
        "editor"
      );
    });

    it("should refresh token on response.statusCode 401", async () => {
      vi.mocked(refreshWorkspaceToken).mockResolvedValue("new-token");

      const authError = { response: { statusCode: 401 } };
      const mockFn = vi
        .fn()
        .mockRejectedValueOnce(authError)
        .mockResolvedValueOnce("success");

      const result = await factoryModule.withTokenRefresh(
        {
          workspace: "my-workspace",
          namespace: "my-namespace",
          role: "editor",
        },
        mockFn
      );

      expect(result).toBe("success");
      expect(refreshWorkspaceToken).toHaveBeenCalled();
    });

    it("should throw non-auth errors", async () => {
      const otherError = new Error("Not found");
      const mockFn = vi.fn().mockRejectedValue(otherError);

      await expect(
        factoryModule.withTokenRefresh(
          {
            workspace: "my-workspace",
            namespace: "my-namespace",
            role: "editor",
          },
          mockFn
        )
      ).rejects.toThrow("Not found");

      expect(refreshWorkspaceToken).not.toHaveBeenCalled();
    });

    it("should throw 403 errors without refresh", async () => {
      const forbiddenError = { statusCode: 403 };
      const mockFn = vi.fn().mockRejectedValue(forbiddenError);

      await expect(
        factoryModule.withTokenRefresh(
          {
            workspace: "my-workspace",
            namespace: "my-namespace",
            role: "editor",
          },
          mockFn
        )
      ).rejects.toEqual(forbiddenError);

      expect(refreshWorkspaceToken).not.toHaveBeenCalled();
    });
  });
});
