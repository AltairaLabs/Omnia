/**
 * Recursive file-tree listing over the operator content API.
 *
 * Several workspace routes (arena projects, arena/skill sources) render a
 * recursive tree of a content subtree. The operator content API exposes only
 * flat directory listings, so this walks it depth-first via getContent. Lives
 * in its own module (not content-api-service) so its getContent call resolves
 * to the imported binding — route tests that mock content-api-service therefore
 * intercept it cleanly.
 */

import type { User } from "@/lib/auth/types";

import { getContent, isContentListing } from "./content-api-service";

/** A node in a content tree. Directories carry children; files carry size. */
export interface ContentTreeNode {
  name: string;
  /** Path relative to the tree root. */
  path: string;
  isDirectory: boolean;
  size?: number;
  modifiedAt: string;
  children?: ContentTreeNode[];
}

export interface ContentTreeOptions {
  /** Skip entries whose name starts with "." (e.g. the .arena versioning dir). */
  skipHidden?: boolean;
}

/**
 * Build a recursive tree of the subtree at relpath. Returns [] when relpath is
 * not a directory. Each directory triggers a further listing (N round trips);
 * workspace content trees are small, so this is acceptable.
 */
export async function listContentTree(
  workspace: string,
  user: User,
  relpath = "",
  options: ContentTreeOptions = {},
): Promise<ContentTreeNode[]> {
  const node = await getContent(workspace, user, relpath);
  if (!isContentListing(node)) {
    return [];
  }

  const nodes: ContentTreeNode[] = [];
  for (const entry of node.entries) {
    if (options.skipHidden && entry.name.startsWith(".")) {
      continue;
    }
    const childPath = relpath ? `${relpath}/${entry.name}` : entry.name;
    if (entry.type === "directory") {
      nodes.push({
        name: entry.name,
        path: childPath,
        isDirectory: true,
        modifiedAt: entry.modifiedAt,
        children: await listContentTree(workspace, user, childPath, options),
      });
    } else {
      nodes.push({
        name: entry.name,
        path: childPath,
        isDirectory: false,
        size: entry.size,
        modifiedAt: entry.modifiedAt,
      });
    }
  }
  return nodes;
}
