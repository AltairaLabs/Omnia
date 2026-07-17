/**
 * Client for the operator's workspace-content API.
 *
 * The dashboard no longer mounts the NFS workspace-content volume directly
 * (#1462). Route handlers call this service, which mints a short-lived RS256
 * identity JWT (carrying the authenticated user's identity + groups) signed
 * with the dashboard's mgmt-plane key and calls the operator content API. The
 * operator verifies the signature and recomputes the workspace role — it never
 * trusts a role claim.
 *
 * Server-only: reads the signing key off disk and never runs in the browser.
 */

import type { User } from "@/lib/auth/types";
import { OperatorApiError, asOperatorError, mintOperatorIdentityToken, operatorBaseURL } from "./operator-identity";

/** A single directory entry, mirroring the Go content.Entry json shape. */
export interface ContentEntry {
  name: string;
  type: "file" | "directory";
  size: number;
  modifiedAt: string;
}

/** Directory listing, mirroring Go content.Listing. */
export interface ContentListing {
  path: string;
  entries: ContentEntry[];
}

/** File content, mirroring Go content.FileContent. */
export interface ContentFile {
  path: string;
  content: string;
  encoding: "utf-8" | "base64";
  size: number;
  modifiedAt: string;
}

/** Result of a write or mkdir, mirroring Go content.WriteResult. */
export interface ContentWriteResult {
  path: string;
  size: number;
  modifiedAt: string;
  directory?: boolean;
}

/** A GET returns either a listing (directory) or file content. */
export type ContentNode = ContentListing | ContentFile;

/** Type guard: the node is a directory listing. */
export function isContentListing(node: ContentNode): node is ContentListing {
  return Array.isArray((node as ContentListing).entries);
}

/** Type guard: the node is file content. */
export function isContentFile(node: ContentNode): node is ContentFile {
  return typeof (node as ContentFile).content === "string";
}

/**
 * Error carrying the operator's HTTP status so route handlers can pass through
 * 404 / 400 / 403 instead of collapsing everything to 500.
 */
export class ContentApiError extends OperatorApiError {
  constructor(message: string, status: number) {
    super(message, status);
    this.name = "ContentApiError";
  }
}

/** Wrap a shared OperatorApiError (config errors) as a ContentApiError so callers only see this file's error type. */
const asContentError = <T>(fn: () => T): T =>
  asOperatorError(fn, (message, status) => new ContentApiError(message, status));

/** Operator content API base URL, without a trailing slash. */
function baseURL(): string {
  return asContentError(() => operatorBaseURL("OPERATOR_CONTENT_API_URL"));
}

function identityToken(workspace: string, user: User): string {
  return asContentError(() => mintOperatorIdentityToken(workspace, user));
}

/** Encode a workspace-relative path, preserving "/" separators between segments. */
function encodeRelPath(relpath: string): string {
  const segs = relpath.split("/").filter(Boolean).map(encodeURIComponent);
  return segs.length > 0 ? "/" + segs.join("/") : "";
}

async function contentRequest<T>(
  method: string,
  workspace: string,
  user: User,
  relpath: string,
  init?: { body?: string },
): Promise<T | undefined> {
  const token = identityToken(workspace, user);
  const url = `${baseURL()}/api/v1/workspaces/${encodeURIComponent(workspace)}/content${encodeRelPath(relpath)}`;
  const res = await fetch(url, {
    method,
    headers: { Authorization: `Bearer ${token}` },
    body: init?.body,
  });
  if (!res.ok) {
    throw new ContentApiError(`content API ${method} ${url} -> ${res.status}`, res.status);
  }
  if (res.status === 204) {
    return undefined;
  }
  return (await res.json()) as T;
}

/** GET a path: returns a directory listing or file content (use the guards). */
export async function getContent(workspace: string, user: User, relpath = ""): Promise<ContentNode> {
  return (await contentRequest<ContentNode>("GET", workspace, user, relpath)) as ContentNode;
}

/** Write (create or overwrite) a file with the given content. */
export async function writeContentFile(
  workspace: string,
  user: User,
  relpath: string,
  content: string,
): Promise<ContentWriteResult> {
  return (await contentRequest<ContentWriteResult>("PUT", workspace, user, relpath, {
    body: content,
  })) as ContentWriteResult;
}

/** Create a directory (and any missing parents) at the given path. */
export async function makeContentDir(
  workspace: string,
  user: User,
  relpath: string,
): Promise<ContentWriteResult> {
  return (await contentRequest<ContentWriteResult>("POST", workspace, user, relpath)) as ContentWriteResult;
}

/** Delete a file or recursively delete a directory. */
export async function deleteContent(workspace: string, user: User, relpath: string): Promise<void> {
  await contentRequest<void>("DELETE", workspace, user, relpath);
}

/**
 * Rename (move) a file or directory. `destRelPath` is workspace-relative, in the
 * same coordinate system as `relpath`. Fails (409) if the destination exists.
 */
export async function moveContent(
  workspace: string,
  user: User,
  relpath: string,
  destRelPath: string,
): Promise<ContentWriteResult> {
  return (await contentRequest<ContentWriteResult>("PATCH", workspace, user, relpath, {
    body: JSON.stringify({ to: destRelPath }),
  })) as ContentWriteResult;
}
