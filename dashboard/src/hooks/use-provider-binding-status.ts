"use client";

import { useMemo } from "react";
import { useProviders } from "./use-providers";
import type { FileTreeNode } from "@/types/arena-project";

export type BindingStatus = "bound" | "stale" | "unbound";

export interface ProviderBindingInfo {
  status: BindingStatus;
  providerName?: string;
  providerNamespace?: string;
  message: string;
}

/**
 * Traverse a file tree and collect all provider file nodes with their paths.
 */
function collectProviderFiles(nodes: FileTreeNode[]): FileTreeNode[] {
  const result: FileTreeNode[] = [];
  for (const node of nodes) {
    if (node.isDirectory && node.children) {
      result.push(...collectProviderFiles(node.children));
    } else if (node.type === "provider") {
      result.push(node);
    }
  }
  return result;
}

/**
 * Cross-references provider file binding annotations against cluster providers
 * to compute binding status for each provider file in the tree.
 *
 * Status values:
 * - **bound** (green): Has annotations AND matching provider exists in cluster
 * - **stale** (blue): Has annotations but provider NOT found in cluster
 * - **unbound** (yellow): No binding annotations
 */
export function useProviderBindingStatus(
  fileTree: FileTreeNode[]
): Map<string, ProviderBindingInfo> {
  const { data: providers } = useProviders();

  return useMemo(() => {
    const statusMap = new Map<string, ProviderBindingInfo>();
    const providerFiles = collectProviderFiles(fileTree);

    // Build a set of existing provider names for fast lookup
    const clusterProviders = new Set(
      (providers ?? []).map((p) => {
        const ns = p.metadata.namespace || "default";
        return `${ns}/${p.metadata.name}`;
      })
    );

    for (const file of providerFiles) {
      const binding = file.providerBinding;

      if (!binding?.providerName) {
        statusMap.set(file.path, {
          status: "unbound",
          message: "Not bound to a cluster provider",
        });
        continue;
      }

      const key = `${binding.providerNamespace || "default"}/${binding.providerName}`;

      if (clusterProviders.has(key)) {
        statusMap.set(file.path, {
          status: "bound",
          providerName: binding.providerName,
          providerNamespace: binding.providerNamespace,
          message: `Bound to ${binding.providerName}`,
        });
      } else {
        statusMap.set(file.path, {
          status: "stale",
          providerName: binding.providerName,
          providerNamespace: binding.providerNamespace,
          message: `Provider "${binding.providerName}" not found in cluster`,
        });
      }
    }

    return statusMap;
  }, [fileTree, providers]);
}
