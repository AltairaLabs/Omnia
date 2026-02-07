import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useProviderBindingStatus } from "./use-provider-binding-status";
import type { FileTreeNode } from "@/types/arena-project";

vi.mock("./use-providers", () => ({
  useProviders: vi.fn(() => ({
    data: [
      {
        metadata: { name: "my-provider", namespace: "default" },
        spec: { type: "openai" },
      },
      {
        metadata: { name: "other-provider", namespace: "production" },
        spec: { type: "anthropic" },
      },
    ],
  })),
}));

describe("useProviderBindingStatus", () => {
  it("should return empty map for empty tree", () => {
    const { result } = renderHook(() => useProviderBindingStatus([]));
    expect(result.current.size).toBe(0);
  });

  it("should return empty map when no provider files exist", () => {
    const tree: FileTreeNode[] = [
      { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, type: "arena" },
      { name: "prompt.prompt.yaml", path: "prompt.prompt.yaml", isDirectory: false, type: "prompt" },
    ];
    const { result } = renderHook(() => useProviderBindingStatus(tree));
    expect(result.current.size).toBe(0);
  });

  it("should mark provider files without binding as unbound", () => {
    const tree: FileTreeNode[] = [
      { name: "test.provider.yaml", path: "test.provider.yaml", isDirectory: false, type: "provider" },
    ];
    const { result } = renderHook(() => useProviderBindingStatus(tree));
    expect(result.current.size).toBe(1);
    const info = result.current.get("test.provider.yaml");
    expect(info?.status).toBe("unbound");
    expect(info?.message).toBe("Not bound to a cluster provider");
  });

  it("should mark bound provider files when provider exists in cluster", () => {
    const tree: FileTreeNode[] = [
      {
        name: "test.provider.yaml",
        path: "test.provider.yaml",
        isDirectory: false,
        type: "provider",
        providerBinding: { providerName: "my-provider", providerNamespace: "default" },
      },
    ];
    const { result } = renderHook(() => useProviderBindingStatus(tree));
    const info = result.current.get("test.provider.yaml");
    expect(info?.status).toBe("bound");
    expect(info?.providerName).toBe("my-provider");
    expect(info?.message).toBe("Bound to my-provider");
  });

  it("should mark stale provider files when provider not in cluster", () => {
    const tree: FileTreeNode[] = [
      {
        name: "test.provider.yaml",
        path: "test.provider.yaml",
        isDirectory: false,
        type: "provider",
        providerBinding: { providerName: "deleted-provider", providerNamespace: "default" },
      },
    ];
    const { result } = renderHook(() => useProviderBindingStatus(tree));
    const info = result.current.get("test.provider.yaml");
    expect(info?.status).toBe("stale");
    expect(info?.message).toContain("not found in cluster");
  });

  it("should traverse nested directories", () => {
    const tree: FileTreeNode[] = [
      {
        name: "providers",
        path: "providers",
        isDirectory: true,
        children: [
          {
            name: "nested.provider.yaml",
            path: "providers/nested.provider.yaml",
            isDirectory: false,
            type: "provider",
            providerBinding: { providerName: "other-provider", providerNamespace: "production" },
          },
        ],
      },
    ];
    const { result } = renderHook(() => useProviderBindingStatus(tree));
    expect(result.current.size).toBe(1);
    const info = result.current.get("providers/nested.provider.yaml");
    expect(info?.status).toBe("bound");
  });

  it("should handle binding with missing namespace (defaults to 'default')", () => {
    const tree: FileTreeNode[] = [
      {
        name: "test.provider.yaml",
        path: "test.provider.yaml",
        isDirectory: false,
        type: "provider",
        providerBinding: { providerName: "my-provider" },
      },
    ];
    const { result } = renderHook(() => useProviderBindingStatus(tree));
    const info = result.current.get("test.provider.yaml");
    expect(info?.status).toBe("bound");
  });
});
