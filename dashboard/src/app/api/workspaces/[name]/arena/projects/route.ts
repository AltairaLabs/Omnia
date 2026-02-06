/**
 * API routes for Arena projects (filesystem-based).
 *
 * GET /api/workspaces/:name/arena/projects - List all projects
 * POST /api/workspaces/:name/arena/projects - Create a new project
 *
 * Projects are stored in the workspace volume at:
 * /workspace-content/{workspace}/{namespace}/arena/projects/{project-id}/
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
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as yaml from "js-yaml";

// Base path for workspace content (configurable via environment)
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

const RESOURCE_TYPE = "ArenaProject";

interface ProjectConfig {
  name: string;
  description?: string;
  tags?: string[];
  createdAt?: string;
  updatedAt?: string;
}

/**
 * Get the projects directory path for a workspace
 */
function getProjectsPath(workspaceName: string, namespace: string): string {
  return path.join(
    WORKSPACE_CONTENT_BASE,
    workspaceName,
    namespace,
    "arena",
    "projects"
  );
}

/**
 * Read project config from config.arena.yaml
 */
async function readProjectConfig(
  projectPath: string,
  projectId: string
): Promise<ArenaProject | null> {
  const configPath = path.join(projectPath, "config.arena.yaml");
  try {
    const content = await fs.readFile(configPath, "utf-8");
    const config = yaml.load(content) as ProjectConfig;
    const stats = await fs.stat(configPath);

    return {
      id: projectId,
      name: config.name || projectId,
      description: config.description,
      tags: config.tags,
      createdAt: config.createdAt || stats.birthtime.toISOString(),
      updatedAt: config.updatedAt || stats.mtime.toISOString(),
    };
  } catch {
    // Config file doesn't exist or is invalid, use defaults
    try {
      const stats = await fs.stat(projectPath);
      return {
        id: projectId,
        name: projectId,
        createdAt: stats.birthtime.toISOString(),
        updatedAt: stats.mtime.toISOString(),
      };
    } catch {
      return null;
    }
  }
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

      const projectsPath = getProjectsPath(name, result.workspace.spec.namespace.name);

      // Ensure projects directory exists
      try {
        await fs.access(projectsPath);
      } catch {
        // Directory doesn't exist, return empty list
        const response: ProjectListResponse = { projects: [] };
        auditSuccess(auditCtx, "list", undefined, { count: 0 });
        return NextResponse.json(response);
      }

      // Read all project directories
      const entries = await fs.readdir(projectsPath, { withFileTypes: true });
      const projects: ArenaProject[] = [];

      for (const entry of entries) {
        if (!entry.isDirectory()) continue;
        // Skip hidden directories
        if (entry.name.startsWith(".")) continue;

        const projectPath = path.join(projectsPath, entry.name);
        const project = await readProjectConfig(projectPath, entry.name);
        if (project) {
          projects.push(project);
        }
      }

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
      return handleK8sError(error, "list projects");
    }
  }
);

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
      const projectsPath = getProjectsPath(name, result.workspace.spec.namespace.name);
      const projectPath = path.join(projectsPath, projectId);

      // Ensure projects directory exists
      await fs.mkdir(projectsPath, { recursive: true });

      // Create project directory
      await fs.mkdir(projectPath, { recursive: true });

      // Create standard subdirectories
      await Promise.all([
        fs.mkdir(path.join(projectPath, "prompts"), { recursive: true }),
        fs.mkdir(path.join(projectPath, "providers"), { recursive: true }),
        fs.mkdir(path.join(projectPath, "scenarios"), { recursive: true }),
        fs.mkdir(path.join(projectPath, "tools"), { recursive: true }),
      ]);

      const now = new Date().toISOString();
      const config: ProjectConfig = {
        name: body.name,
        description: body.description,
        tags: body.tags,
        createdAt: now,
        updatedAt: now,
      };

      // Write config.arena.yaml
      const configPath = path.join(projectPath, "config.arena.yaml");
      const configContent = yaml.dump(config, { lineWidth: -1 });
      await fs.writeFile(configPath, configContent, "utf-8");

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
      return handleK8sError(error, "create project");
    }
  }
);
