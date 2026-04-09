import { describe, it, expect, beforeEach, vi } from "vitest";
import type { Workspace } from "@/types/workspace";

// These need to be defined before the mock factory runs
const mockGetClusterCustomObject = vi.fn();
const mockListClusterCustomObject = vi.fn();
const mockPatchClusterCustomObject = vi.fn();
const mockLoadFromCluster = vi.fn();
const mockLoadFromDefault = vi.fn();

// Track how many times KubeConfig is instantiated
let kubeConfigInstances = 0;

// Mock @kubernetes/client-node
vi.mock("@kubernetes/client-node", () => {
  return {
    KubeConfig: class {
      constructor() {
        kubeConfigInstances++;
      }
      loadFromCluster() {
        return mockLoadFromCluster();
      }
      loadFromDefault() {
        return mockLoadFromDefault();
      }
      getCurrentCluster() {
        return { server: "https://mock-k8s:6443", name: "mock" };
      }
      makeApiClient() {
        return {
          getClusterCustomObject: mockGetClusterCustomObject,
          listClusterCustomObject: mockListClusterCustomObject,
          patchClusterCustomObject: mockPatchClusterCustomObject,
        };
      }
    },
    CustomObjectsApi: class {},
    Watch: class {},
  };
});

import {
  getWorkspace,
  listWorkspaces,
  patchWorkspace,
  watchWorkspaces,
  getWorkspaceWatchPath,
  resetWorkspaceClient,
} from "./workspace-client";

describe("workspace-client", () => {
  const mockWorkspace: Workspace = {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Workspace",
    metadata: {
      name: "test-workspace",
      creationTimestamp: "2024-01-15T10:00:00Z",
    },
    spec: {
      displayName: "Test Workspace",
      description: "A test workspace",
      environment: "development",
      namespace: {
        name: "test-ns",
        create: true,
      },
      roleBindings: [
        {
          groups: ["developers@example.com"],
          role: "editor",
        },
      ],
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    resetWorkspaceClient();
    kubeConfigInstances = 0;

    // Default: loadFromCluster succeeds
    mockLoadFromCluster.mockImplementation(() => {});
    mockLoadFromDefault.mockImplementation(() => {});
  });

  describe("getWorkspace", () => {
    it("should return workspace when found", async () => {
      mockGetClusterCustomObject.mockResolvedValue(mockWorkspace);

      const result = await getWorkspace("test-workspace");

      expect(result).toEqual(mockWorkspace);
      expect(mockGetClusterCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        plural: "workspaces",
        name: "test-workspace",
      });
    });

    it("should return null when workspace not found (statusCode 404)", async () => {
      mockGetClusterCustomObject.mockRejectedValue({ statusCode: 404 });

      const result = await getWorkspace("nonexistent");

      expect(result).toBeNull();
    });

    it("should return null when workspace not found (response.statusCode 404)", async () => {
      mockGetClusterCustomObject.mockRejectedValue({
        response: { statusCode: 404 },
      });

      const result = await getWorkspace("nonexistent");

      expect(result).toBeNull();
    });

    it("should throw for non-404 errors", async () => {
      const error = new Error("Connection failed");
      mockGetClusterCustomObject.mockRejectedValue(error);

      await expect(getWorkspace("test-workspace")).rejects.toThrow(
        "Connection failed"
      );
    });

    it("should reuse singleton client", async () => {
      mockGetClusterCustomObject.mockResolvedValue(mockWorkspace);

      await getWorkspace("workspace-1");
      await getWorkspace("workspace-2");

      // KubeConfig should only be instantiated once (singleton pattern)
      expect(kubeConfigInstances).toBe(1);
    });

    it("should fall back to loadFromDefault when loadFromCluster fails", async () => {
      mockLoadFromCluster.mockImplementation(() => {
        throw new Error("Not in cluster");
      });
      mockGetClusterCustomObject.mockResolvedValue(mockWorkspace);

      await getWorkspace("test-workspace");

      expect(mockLoadFromCluster).toHaveBeenCalled();
      expect(mockLoadFromDefault).toHaveBeenCalled();
    });
  });

  describe("listWorkspaces", () => {
    it("should return list of workspaces", async () => {
      mockListClusterCustomObject.mockResolvedValue({
        items: [mockWorkspace],
      });

      const result = await listWorkspaces();

      expect(result).toEqual([mockWorkspace]);
      expect(mockListClusterCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        plural: "workspaces",
        labelSelector: undefined,
      });
    });

    it("should pass label selector when provided", async () => {
      mockListClusterCustomObject.mockResolvedValue({
        items: [mockWorkspace],
      });

      await listWorkspaces("env=production");

      expect(mockListClusterCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        plural: "workspaces",
        labelSelector: "env=production",
      });
    });

    it("should return empty array when items is undefined", async () => {
      mockListClusterCustomObject.mockResolvedValue({});

      const result = await listWorkspaces();

      expect(result).toEqual([]);
    });

    it("should return empty array for empty list", async () => {
      mockListClusterCustomObject.mockResolvedValue({ items: [] });

      const result = await listWorkspaces();

      expect(result).toEqual([]);
    });
  });

  describe("patchWorkspace", () => {
    it("should return patched workspace on success", async () => {
      const patchedWorkspace: Workspace = {
        ...mockWorkspace,
        spec: {
          ...mockWorkspace.spec,
          displayName: "Updated Workspace",
        },
      };
      mockPatchClusterCustomObject.mockResolvedValue(patchedWorkspace);

      const result = await patchWorkspace("test-workspace", {
        displayName: "Updated Workspace",
      });

      expect(result).toEqual(patchedWorkspace);
      expect(mockPatchClusterCustomObject).toHaveBeenCalledWith({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        plural: "workspaces",
        name: "test-workspace",
        body: { spec: { displayName: "Updated Workspace" } },
      });
    });

    it("should return null on error", async () => {
      mockPatchClusterCustomObject.mockRejectedValue(
        new Error("Patch failed")
      );

      const result = await patchWorkspace("test-workspace", {
        displayName: "Updated Workspace",
      });

      expect(result).toBeNull();
    });

    it("should return null when client is unavailable", async () => {
      // Reset so the next call gets a fresh null client
      resetWorkspaceClient();

      // Override getCurrentCluster to return null server
      const k8s = await import("@kubernetes/client-node");
      const origGetCurrentCluster = k8s.KubeConfig.prototype.getCurrentCluster;
      // @ts-expect-error - patching prototype for test
      k8s.KubeConfig.prototype.getCurrentCluster = () => ({ server: "" });

      const result = await patchWorkspace("test-workspace", {
        displayName: "Updated Workspace",
      });

      expect(result).toBeNull();
      expect(mockPatchClusterCustomObject).not.toHaveBeenCalled();

      // Restore
      k8s.KubeConfig.prototype.getCurrentCluster = origGetCurrentCluster;
      resetWorkspaceClient();
    });
  });

  describe("watchWorkspaces", () => {
    it("should return a Watch instance", async () => {
      const watch = await watchWorkspaces();

      expect(watch).toBeDefined();
    });

    it("should fall back to loadFromDefault when loadFromCluster fails", async () => {
      // Reset state for this test
      kubeConfigInstances = 0;
      mockLoadFromCluster.mockClear();
      mockLoadFromDefault.mockClear();

      mockLoadFromCluster.mockImplementation(() => {
        throw new Error("Not in cluster");
      });

      await watchWorkspaces();

      expect(mockLoadFromCluster).toHaveBeenCalled();
      expect(mockLoadFromDefault).toHaveBeenCalled();
    });
  });

  describe("getWorkspaceWatchPath", () => {
    it("should return correct API path", () => {
      const path = getWorkspaceWatchPath();

      expect(path).toBe("/apis/omnia.altairalabs.ai/v1alpha1/workspaces");
    });
  });

  describe("resetWorkspaceClient", () => {
    it("should reset the singleton client", async () => {
      mockGetClusterCustomObject.mockResolvedValue(mockWorkspace);

      // First call creates client
      await getWorkspace("workspace-1");
      expect(kubeConfigInstances).toBe(1);

      // Reset the client
      resetWorkspaceClient();

      // Second call should create a new client
      await getWorkspace("workspace-2");
      expect(kubeConfigInstances).toBe(2);
    });
  });
});
