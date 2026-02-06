/**
 * Generic CRD CRUD operations using workspace-scoped ServiceAccount tokens.
 *
 * Provides type-safe operations for Omnia CRDs (AgentRuntime, PromptPack, etc.)
 * using the workspace K8s client factory for authentication.
 *
 * Usage:
 * ```typescript
 * const agents = await listCrd<AgentRuntime>({
 *   workspace: "my-workspace",
 *   namespace: "workspace-ns",
 *   role: "editor",
 * }, "agentruntimes");
 * ```
 */

import * as k8s from "@kubernetes/client-node";
import {
  getWorkspaceCustomObjectsApi,
  getWorkspaceCoreApi,
  getWorkspaceKubeConfig,
  withTokenRefresh,
  type WorkspaceClientOptions,
} from "./workspace-k8s-client-factory";

const CRD_GROUP = "omnia.altairalabs.ai";
const CRD_VERSION = "v1alpha1";

/**
 * List CRD resources in a workspace namespace.
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name (e.g., "agentruntimes")
 * @returns Array of CRD resources
 */
export async function listCrd<T>(
  options: WorkspaceClientOptions,
  plural: string
): Promise<T[]> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    const result = await api.listNamespacedCustomObject({
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace: options.namespace,
      plural,
    });
    const list = result as { items?: T[] };
    return (list.items || []) as T[];
  });
}

/**
 * Get a single CRD resource by name.
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name
 * @param name - Resource name
 * @returns The resource or null if not found
 */
export async function getCrd<T>(
  options: WorkspaceClientOptions,
  plural: string,
  name: string
): Promise<T | null> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    try {
      const result = await api.getNamespacedCustomObject({
        group: CRD_GROUP,
        version: CRD_VERSION,
        namespace: options.namespace,
        plural,
        name,
      });
      return result as T;
    } catch (error) {
      if (isNotFoundError(error)) {
        return null;
      }
      throw error;
    }
  });
}

/**
 * Create a new CRD resource.
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name
 * @param resource - The resource to create (must include metadata.name)
 * @returns The created resource
 */
export async function createCrd<T>(
  options: WorkspaceClientOptions,
  plural: string,
  resource: T
): Promise<T> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    const result = await api.createNamespacedCustomObject({
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace: options.namespace,
      plural,
      body: resource,
    });
    return result as T;
  });
}

/**
 * Update an existing CRD resource (full replacement).
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name
 * @param name - Resource name
 * @param resource - The updated resource
 * @returns The updated resource
 */
export async function updateCrd<T>(
  options: WorkspaceClientOptions,
  plural: string,
  name: string,
  resource: T
): Promise<T> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    const result = await api.replaceNamespacedCustomObject({
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace: options.namespace,
      plural,
      name,
      body: resource,
    });
    return result as T;
  });
}

/**
 * Patch a CRD resource (partial update).
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name
 * @param name - Resource name
 * @param patch - The patch to apply (merge patch format)
 * @returns The patched resource
 */
export async function patchCrd<T>(
  options: WorkspaceClientOptions,
  plural: string,
  name: string,
  patch: Record<string, unknown>
): Promise<T> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    const result = await api.patchNamespacedCustomObject({
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace: options.namespace,
      plural,
      name,
      body: patch,
    });
    return result as T;
  });
}

/**
 * Delete a CRD resource.
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name
 * @param name - Resource name
 */
export async function deleteCrd(
  options: WorkspaceClientOptions,
  plural: string,
  name: string
): Promise<void> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    await api.deleteNamespacedCustomObject({
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace: options.namespace,
      plural,
      name,
    });
  });
}

type LogEntry = { timestamp: string; message: string; container?: string };

/**
 * Parse a single log line into a LogEntry.
 * K8s log lines with timestamps have format: "2024-01-01T10:00:00.123456789Z message"
 */
function parseLogLine(line: string, containerName: string): LogEntry {
  // Find the first space after timestamp (timestamp ends at first whitespace)
  const spaceIndex = line.indexOf(" ");
  if (spaceIndex > 0) {
    const possibleTimestamp = line.substring(0, spaceIndex);
    // Check if it looks like an ISO timestamp (starts with YYYY-MM-DD)
    if (/^\d{4}-\d{2}-\d{2}T/.test(possibleTimestamp)) {
      return {
        timestamp: possibleTimestamp,
        message: line.substring(spaceIndex + 1),
        container: containerName,
      };
    }
  }
  return {
    timestamp: new Date().toISOString(),
    message: line,
    container: containerName,
  };
}

/**
 * Fetch and parse logs for a single container.
 */
async function fetchContainerLogs(
  coreApi: k8s.CoreV1Api,
  namespace: string,
  podName: string,
  containerName: string,
  tailLines?: number,
  sinceSeconds?: number
): Promise<LogEntry[]> {
  try {
    const logResponse = await coreApi.readNamespacedPodLog({
      namespace,
      name: podName,
      container: containerName,
      tailLines,
      sinceSeconds,
      timestamps: true,
    });

    const logText = typeof logResponse === "string" ? logResponse : "";
    return logText
      .split("\n")
      .filter(Boolean)
      .map((line) => parseLogLine(line, containerName));
  } catch (error) {
    console.warn(`Failed to get logs from ${podName}/${containerName}:`, error);
    return [];
  }
}

/**
 * Get logs from a pod associated with a CRD resource.
 *
 * @param options - Workspace client options
 * @param labelSelector - Label selector to find pods (e.g., "app.kubernetes.io/instance=my-agent")
 * @param tailLines - Number of lines to return from the end
 * @param sinceSeconds - Only return logs newer than this many seconds
 * @param container - Container name (if pod has multiple containers)
 * @returns Array of log entries
 */
export async function getPodLogs(
  options: WorkspaceClientOptions,
  labelSelector: string,
  tailLines?: number,
  sinceSeconds?: number,
  container?: string
): Promise<LogEntry[]> {
  return withTokenRefresh(options, async () => {
    const coreApi = await getWorkspaceCoreApi(options);

    const podsResponse = await coreApi.listNamespacedPod({
      namespace: options.namespace,
      labelSelector,
    });

    const pods = podsResponse.items || [];
    if (pods.length === 0) {
      return [];
    }

    const logPromises: Promise<LogEntry[]>[] = [];

    for (const pod of pods) {
      const podName = pod.metadata?.name;
      if (!podName) continue;

      const containers = container
        ? [container]
        : (pod.spec?.containers || []).map((c) => c.name);

      for (const containerName of containers) {
        logPromises.push(
          fetchContainerLogs(coreApi, options.namespace, podName, containerName, tailLines, sinceSeconds)
        );
      }
    }

    const logArrays = await Promise.all(logPromises);
    const logs = logArrays.flat();
    logs.sort((a, b) => a.timestamp.localeCompare(b.timestamp));
    return logs;
  });
}

type K8sEventResult = {
  type: "Normal" | "Warning";
  reason: string;
  message: string;
  firstTimestamp: string;
  lastTimestamp: string;
  count: number;
  source: { component?: string; host?: string };
  involvedObject: { kind: string; name: string; namespace?: string };
};

/**
 * Get Kubernetes events related to a resource and its associated pods.
 *
 * This fetches events for both the CRD resource itself (e.g., AgentRuntime)
 * and any pods that belong to it (via app.kubernetes.io/instance label).
 * This provides visibility into pod-level issues like CrashLoopBackOff,
 * ImagePullBackOff, OOMKilled, etc.
 *
 * @param options - Workspace client options
 * @param resourceKind - Kind of the resource (e.g., "AgentRuntime")
 * @param resourceName - Name of the resource
 * @returns Array of events from both the resource and its pods
 */
export async function getResourceEvents(
  options: WorkspaceClientOptions,
  resourceKind: string,
  resourceName: string
): Promise<K8sEventResult[]> {
  return withTokenRefresh(options, async () => {
    const coreApi = await getWorkspaceCoreApi(options);

    // Helper to safely convert timestamp to ISO string
    // K8s client may return Date objects or strings depending on the field
    const toISOString = (ts: Date | string | undefined): string => {
      if (!ts) return "";
      if (typeof ts === "string") return ts;
      if (ts instanceof Date) return ts.toISOString();
      // Handle any other object with toISOString method
      if (typeof (ts as { toISOString?: () => string }).toISOString === "function") {
        return (ts as { toISOString: () => string }).toISOString();
      }
      return String(ts);
    };

    // Helper to map K8s event to our result type
    const mapEvent = (event: k8s.CoreV1Event): K8sEventResult => ({
      type: (event.type as "Normal" | "Warning") || "Normal",
      reason: event.reason || "",
      message: event.message || "",
      firstTimestamp:
        toISOString(event.firstTimestamp) ||
        toISOString(event.eventTime) ||
        "",
      lastTimestamp:
        toISOString(event.lastTimestamp) ||
        toISOString(event.eventTime) ||
        "",
      count: event.count || 1,
      source: {
        component: event.source?.component,
        host: event.source?.host,
      },
      involvedObject: {
        kind: event.involvedObject?.kind || "",
        name: event.involvedObject?.name || "",
        namespace: event.involvedObject?.namespace,
      },
    });

    // 1. Fetch events for the CRD resource itself
    const resourceFieldSelector = `involvedObject.kind=${resourceKind},involvedObject.name=${resourceName}`;
    const resourceEventsResponse = await coreApi.listNamespacedEvent({
      namespace: options.namespace,
      fieldSelector: resourceFieldSelector,
    });
    const resourceEvents = (resourceEventsResponse.items || []).map(mapEvent);

    // 2. Find pods belonging to this resource and fetch their events
    const podEvents: K8sEventResult[] = [];
    try {
      // Pods are labeled with app.kubernetes.io/instance=<resourceName>
      const podsResponse = await coreApi.listNamespacedPod({
        namespace: options.namespace,
        labelSelector: `app.kubernetes.io/instance=${resourceName}`,
      });

      const pods = podsResponse.items || [];

      // Fetch events for each pod
      for (const pod of pods) {
        const podName = pod.metadata?.name;
        if (!podName) continue;

        const podFieldSelector = `involvedObject.kind=Pod,involvedObject.name=${podName}`;
        const podEventsResponse = await coreApi.listNamespacedEvent({
          namespace: options.namespace,
          fieldSelector: podFieldSelector,
        });

        const mappedPodEvents = (podEventsResponse.items || []).map(mapEvent);
        podEvents.push(...mappedPodEvents);
      }
    } catch (error) {
      // Log but don't fail if pod events can't be fetched
      console.warn(`Failed to fetch pod events for ${resourceName}:`, error);
    }

    // 3. Combine and deduplicate events (by involvedObject.name + reason + message)
    const allEvents = [...resourceEvents, ...podEvents];
    const seen = new Set<string>();
    const deduped: K8sEventResult[] = [];

    for (const event of allEvents) {
      const key = `${event.involvedObject.kind}:${event.involvedObject.name}:${event.reason}:${event.message}`;
      if (!seen.has(key)) {
        seen.add(key);
        deduped.push(event);
      }
    }

    return deduped;
  });
}

/**
 * List shared CRDs using system-level access.
 * Used for cluster-wide resources like ToolRegistries and Providers.
 *
 * @param plural - CRD plural name
 * @param namespace - Namespace to list from (usually omnia-system)
 * @returns Array of CRD resources
 */
export async function listSharedCrd<T>(
  plural: string,
  namespace: string
): Promise<T[]> {
  const kc = new k8s.KubeConfig();

  try {
    kc.loadFromCluster();
  } catch {
    kc.loadFromDefault();
  }

  const api = kc.makeApiClient(k8s.CustomObjectsApi);

  const result = await api.listNamespacedCustomObject({
    group: CRD_GROUP,
    version: CRD_VERSION,
    namespace,
    plural,
  });

  const list = result as { items?: T[] };
  return (list.items || []) as T[];
}

/**
 * Get a shared CRD resource using system-level access.
 *
 * @param plural - CRD plural name
 * @param namespace - Namespace to get from
 * @param name - Resource name
 * @returns The resource or null if not found
 */
export async function getSharedCrd<T>(
  plural: string,
  namespace: string,
  name: string
): Promise<T | null> {
  const kc = new k8s.KubeConfig();

  try {
    kc.loadFromCluster();
  } catch {
    kc.loadFromDefault();
  }

  const api = kc.makeApiClient(k8s.CustomObjectsApi);

  try {
    const result = await api.getNamespacedCustomObject({
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace,
      plural,
      name,
    });
    return result as T;
  } catch (error) {
    if (isNotFoundError(error)) {
      return null;
    }
    throw error;
  }
}

/**
 * Extract files from a tar.gz buffer.
 * @param buffer - The gzipped tar buffer
 * @returns Record of file paths to content
 */
async function extractTarGz(buffer: Buffer): Promise<Record<string, string>> {
  const { gunzipSync } = await import("zlib");
  const tar = await import("tar-stream");

  const decompressed = gunzipSync(buffer);
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
          const cleanPath = header.name.replace(/^\.?\/?/, "");
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
      resolve({});
    });

    extract.end(decompressed);
  });
}

/**
 * Get the content of a ConfigMap (for PromptPack/Arena content).
 * Supports both binaryData (tar.gz archive) and raw data keys.
 *
 * @param options - Workspace client options
 * @param configMapName - Name of the ConfigMap
 * @returns The ConfigMap data or null if not found
 */
export async function getConfigMapContent(
  options: WorkspaceClientOptions,
  configMapName: string
): Promise<Record<string, string> | null> {
  return withTokenRefresh(options, async () => {
    const coreApi = await getWorkspaceCoreApi(options);

    try {
      const result = await coreApi.readNamespacedConfigMap({
        namespace: options.namespace,
        name: configMapName,
      });

      // Check for binaryData with tar.gz archive first
      const binaryData = result.binaryData;
      if (binaryData) {
        // Look for any tar.gz file in binaryData
        const tarGzKey = Object.keys(binaryData).find(
          (key) => key.endsWith(".tar.gz") || key.endsWith(".tgz")
        );
        if (tarGzKey) {
          const base64Content = binaryData[tarGzKey];
          const buffer = Buffer.from(base64Content, "base64");
          const files = await extractTarGz(buffer);
          if (Object.keys(files).length > 0) {
            return files;
          }
        }
      }

      // Fall back to raw data keys
      return result.data || null;
    } catch (error) {
      if (isNotFoundError(error)) {
        return null;
      }
      throw error;
    }
  });
}

/**
 * Scale a deployment associated with a CRD.
 *
 * @param options - Workspace client options
 * @param deploymentName - Name of the deployment
 * @param replicas - Desired replica count
 */
export async function scaleDeployment(
  options: WorkspaceClientOptions,
  deploymentName: string,
  replicas: number
): Promise<void> {
  return withTokenRefresh(options, async () => {
    const kc = await getWorkspaceKubeConfig(options);
    const appsApi = kc.makeApiClient(k8s.AppsV1Api);

    await appsApi.patchNamespacedDeploymentScale({
      namespace: options.namespace,
      name: deploymentName,
      body: {
        spec: {
          replicas,
        },
      },
    });
  });
}

// Helper functions

/**
 * Extract status code from various Kubernetes client error formats.
 */
function extractStatusCode(error: unknown): number | null {
  if (typeof error !== "object" || error === null) {
    return null;
  }

  const err = error as Record<string, unknown>;

  // Direct statusCode property
  if (typeof err.statusCode === "number") {
    return err.statusCode;
  }

  // Response statusCode
  if (err.response && typeof (err.response as Record<string, unknown>).statusCode === "number") {
    return (err.response as Record<string, unknown>).statusCode as number;
  }

  // Kubernetes client error format: "HTTP-Code: 404" in message
  if (typeof err.message === "string" && /HTTP-Code:\s*(\d+)/.test(err.message)) {
    const match = /HTTP-Code:\s*(\d+)/.exec(err.message);
    if (match) {
      return Number.parseInt(match[1], 10);
    }
  }

  // Kubernetes API response body
  if (typeof err.body === "string") {
    try {
      const parsed = JSON.parse(err.body) as Record<string, unknown>;
      if (typeof parsed.code === "number") {
        return parsed.code;
      }
    } catch {
      // Not JSON, ignore
    }
  } else if (err.body && typeof (err.body as Record<string, unknown>).code === "number") {
    return (err.body as Record<string, unknown>).code as number;
  }

  return null;
}

function isNotFoundError(error: unknown): boolean {
  return extractStatusCode(error) === 404;
}

/**
 * Extract error message from K8s API error.
 */
export function extractK8sErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === "object" && error !== null) {
    if ("body" in error && typeof (error as { body: unknown }).body === "object") {
      const body = (error as { body: { message?: string } }).body;
      if (body?.message) {
        return body.message;
      }
    }
    if ("message" in error && typeof (error as { message: unknown }).message === "string") {
      return (error as { message: string }).message;
    }
  }
  return String(error);
}

/**
 * Check if an error indicates insufficient permissions.
 */
export function isForbiddenError(error: unknown): boolean {
  return extractStatusCode(error) === 403;
}
