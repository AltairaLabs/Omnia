/**
 * API route for getting individual file content from an Arena config.
 *
 * GET /api/workspaces/:name/arena/configs/:configName/content/file?path=... - Get file content
 *
 * Returns the content of a specific file from the config's source.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_CONFIGS,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import { getConfigMapContent } from "@/lib/k8s/crd-operations";
import { gunzipSync } from "zlib";
import * as tar from "tar-stream";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaConfig, ArenaSource } from "@/types/arena";

type RouteParams = { name: string; configName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaConfig";

/** Fetch and extract tar.gz artifact from URL */
async function fetchArtifactContent(url: string): Promise<Record<string, string> | null> {
  try {
    // Rewrite localhost URLs to use internal Kubernetes service
    // eslint-disable-next-line sonarjs/no-clear-text-protocols -- internal K8s service communication
    const K8S_SERVICE_URL = "http://omnia-controller-manager.omnia-system:8082";
    let fetchUrl = url;
    if (url.includes("localhost:8082")) {
      fetchUrl = url.replace("http://localhost:8082", K8S_SERVICE_URL);
    }
    const response = await fetch(fetchUrl);
    if (!response.ok) {
      console.error(`Failed to fetch artifact: ${response.status} ${response.statusText}`);
      return null;
    }

    const buffer = Buffer.from(await response.arrayBuffer());

    // Decompress gzip
    const decompressed = gunzipSync(buffer);

    // Extract tar
    const files: Record<string, string> = {};
    const extract = tar.extract();

    return new Promise((resolve) => {
      extract.on("entry", (header, stream, next) => {
        const chunks: Buffer[] = [];

        stream.on("data", (chunk: Buffer) => {
          chunks.push(chunk);
        });

        stream.on("end", () => {
          if (header.type === "file" && header.name) {
            const content = Buffer.concat(chunks).toString("utf-8");
            // Remove leading ./ or / from path
            const cleanPath = header.name.replace(/^\.?\//, "");
            if (cleanPath && !cleanPath.endsWith("/")) {
              files[cleanPath] = content;
            }
          }
          next();
        });

        stream.resume();
      });

      extract.on("finish", () => {
        resolve(files);
      });

      extract.on("error", (err) => {
        console.error("Tar extraction error:", err);
        resolve(null);
      });

      extract.end(decompressed);
    });
  } catch (err) {
    console.error("Error fetching artifact:", err);
    return null;
  }
}

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, configName } = await context.params;
    const filePath = request.nextUrl.searchParams.get("path");
    let auditCtx;

    if (!filePath) {
      return NextResponse.json(
        { error: "Missing 'path' query parameter" },
        { status: 400 }
      );
    }

    try {
      const result = await getWorkspaceResource<ArenaConfig>(
        name,
        access.role!,
        CRD_ARENA_CONFIGS,
        configName,
        "Arena config"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      // Get the source reference from the config
      const sourceRef = result.resource.spec?.sourceRef?.name;
      if (!sourceRef) {
        auditSuccess(auditCtx, "get", configName, { subresource: "file", path: filePath });
        return NextResponse.json(
          { error: "No source configured" },
          { status: 404 }
        );
      }

      // Fetch the ArenaSource to get the ConfigMap reference
      const sourceResult = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceRef,
        "Arena source"
      );
      if (!sourceResult.ok) {
        auditSuccess(auditCtx, "get", configName, { subresource: "file", path: filePath });
        return NextResponse.json(
          { error: "Source not found" },
          { status: 404 }
        );
      }

      // Try to get content from artifact URL first (for git/oci/s3 sources)
      // Fall back to ConfigMap if artifact not available
      let packageFiles: Record<string, string> | null = null;

      const artifactUrl = sourceResult.resource.status?.artifact?.url;
      if (artifactUrl) {
        packageFiles = await fetchArtifactContent(artifactUrl);
      }

      // Fall back to ConfigMap if artifact fetch failed or not available
      if (!packageFiles) {
        const configMapName = sourceResult.resource.spec?.configMap?.name;
        if (configMapName) {
          packageFiles = await getConfigMapContent(sourceResult.clientOptions, configMapName);
        }
      }

      if (!packageFiles) {
        auditSuccess(auditCtx, "get", configName, { subresource: "file", path: filePath });
        return NextResponse.json(
          { error: "No content available" },
          { status: 404 }
        );
      }

      // Get the specific file content
      const content = packageFiles[filePath];
      if (content === undefined) {
        auditSuccess(auditCtx, "get", configName, { subresource: "file", path: filePath });
        return NextResponse.json(
          { error: `File not found: ${filePath}` },
          { status: 404 }
        );
      }

      auditSuccess(auditCtx, "get", configName, {
        subresource: "file",
        path: filePath,
        size: content.length,
      });

      return NextResponse.json({
        path: filePath,
        content,
        size: content.length,
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", configName, error, 500);
      }
      return handleK8sError(error, "get file content for this arena config");
    }
  }
);
