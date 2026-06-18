/**
 * API routes for Arena projects (workspace-content based).
 *
 * GET /api/workspaces/:name/arena/projects - List all projects
 * POST /api/workspaces/:name/arena/projects - Create a new project
 *
 * Projects live in the workspace content at the relative path
 * `arena/projects/{project-id}/`. Content is served by the operator's
 * authenticated content API (the dashboard no longer mounts the NFS
 * workspace-content volume directly — see #1462); the operator resolves the
 * workspace's namespace and confines access.
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  validateWorkspace,
  handleK8sError,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaProject, ProjectListResponse, ProjectCreateRequest } from "@/types/arena-project";
import {
  ContentApiError,
  getContent,
  isContentFile,
  isContentListing,
  makeContentDir,
  writeContentFile,
} from "@/lib/data/content-api-service";
import { contentErrorResponse } from "@/lib/data/content-api-response";
import * as yaml from "js-yaml";

const RESOURCE_TYPE = "ArenaProject";
const PROJECTS_REL_PATH = "arena/projects";
const PROJECT_CONFIG_FILE = "config.arena.yaml";

interface ProjectConfig {
  name: string;
  description?: string;
  tags?: string[];
  createdAt?: string;
  updatedAt?: string;
}

/** The content relpath for a project's directory. */
function projectRelPath(projectId: string): string {
  return `${PROJECTS_REL_PATH}/${projectId}`;
}

/**
 * Read a project's config.arena.yaml via the content API, returning project
 * metadata. Falls back to the project id as the name if the config is absent
 * or invalid.
 */
async function readProjectConfig(
  workspace: string,
  user: User,
  projectId: string,
): Promise<ArenaProject> {
  const configPath = `${projectRelPath(projectId)}/${PROJECT_CONFIG_FILE}`;
  let config: ProjectConfig | undefined;
  let modifiedAt = "";
  try {
    const node = await getContent(workspace, user, configPath);
    if (isContentFile(node)) {
      config = yaml.load(node.content) as ProjectConfig;
      modifiedAt = node.modifiedAt;
    }
  } catch (error) {
    if (!(error instanceof ContentApiError && error.status === 404)) {
      throw error;
    }
  }

  return {
    id: projectId,
    name: config?.name || projectId,
    description: config?.description,
    tags: config?.tags,
    createdAt: config?.createdAt || modifiedAt,
    updatedAt: config?.updatedAt || modifiedAt,
  };
}

/**
 * Generate a unique project ID
 */
function generateProjectId(name: string): string {
  const slug = name
    .toLowerCase()
    .replaceAll(/[^a-z0-9]+/g, "-")
    .replaceAll(/(?:^-)|(?:-$)/g, "")
    .slice(0, 40);
  const timestamp = Date.now().toString(36);
  return `${slug}-${timestamp}`;
}

/**
 * GET /api/workspaces/:name/arena/projects
 *
 * List all projects in the workspace.
 */
export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        RESOURCE_TYPE
      );

      const projects = await listProjects(name, user);

      // Sort by updatedAt (most recent first)
      projects.sort((a, b) =>
        new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
      );

      const response: ProjectListResponse = { projects };
      auditSuccess(auditCtx, "list", undefined, { count: projects.length });
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to list projects");
      }
      return handleK8sError(error, "list projects");
    }
  }
);

/**
 * List projects under arena/projects. A missing projects directory yields an
 * empty list (404 from the content API is treated as "no projects yet").
 */
async function listProjects(workspace: string, user: User): Promise<ArenaProject[]> {
  let listing;
  try {
    listing = await getContent(workspace, user, PROJECTS_REL_PATH);
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return [];
    }
    throw error;
  }

  if (!isContentListing(listing)) {
    return [];
  }

  const projects: ArenaProject[] = [];
  for (const entry of listing.entries) {
    if (entry.type !== "directory") continue;
    if (entry.name.startsWith(".")) continue;
    projects.push(await readProjectConfig(workspace, user, entry.name));
  }
  return projects;
}

/**
 * POST /api/workspaces/:name/arena/projects
 *
 * Create a new project.
 */
export const POST = withWorkspaceAccess(
  "editor",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;
    let projectId = "";

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        RESOURCE_TYPE
      );

      const body = (await request.json()) as ProjectCreateRequest;

      if (!body.name || typeof body.name !== "string") {
        return NextResponse.json(
          { error: "Bad Request", message: "Project name is required" },
          { status: 400 }
        );
      }

      projectId = generateProjectId(body.name);
      const basePath = projectRelPath(projectId);

      // Create standard subdirectories (the project dir and parents are
      // created implicitly by the operator's recursive mkdir).
      await Promise.all([
        makeContentDir(name, user, `${basePath}/prompts`),
        makeContentDir(name, user, `${basePath}/providers`),
        makeContentDir(name, user, `${basePath}/scenarios`),
        makeContentDir(name, user, `${basePath}/tools`),
      ]);

      const now = new Date().toISOString();
      const config: ProjectConfig = {
        name: body.name,
        description: body.description,
        tags: body.tags,
        createdAt: now,
        updatedAt: now,
      };

      const configContent = yaml.dump(config, { lineWidth: -1 });
      await writeContentFile(name, user, `${basePath}/${PROJECT_CONFIG_FILE}`, configContent);

      const project: ArenaProject = {
        id: projectId,
        name: body.name,
        description: body.description,
        tags: body.tags,
        createdAt: now,
        updatedAt: now,
      };

      auditSuccess(auditCtx, "create", projectId);
      return NextResponse.json(project, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", projectId, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to create project");
      }
      return handleK8sError(error, "create project");
    }
  }
);
