/**
 * Structured audit logging for dashboard operations.
 *
 * Provides JSON-formatted audit logs for CRD operations, authentication events,
 * and other security-relevant actions. Designed for integration with log
 * aggregation systems (stdout → Loki/CloudWatch/etc.).
 *
 * Usage:
 * ```typescript
 * import { logAudit } from "@/lib/audit/logger";
 *
 * logAudit({
 *   action: "create",
 *   resourceType: "AgentRuntime",
 *   resourceName: "my-agent",
 *   workspace: "production",
 *   namespace: "workspace-prod",
 *   user: "user@example.com",
 *   result: "success",
 * });
 * ```
 */

/**
 * Audit action types for CRD and resource operations.
 */
export type AuditAction =
  | "list"
  | "get"
  | "create"
  | "update"
  | "patch"
  | "delete"
  | "scale";

/**
 * Result of an audited operation.
 */
export type AuditResult = "success" | "denied" | "error" | "not_found";

/**
 * Structured audit log entry.
 */
export interface AuditEntry {
  /** ISO timestamp of the event */
  timestamp: string;
  /** Log level indicator for filtering */
  level: "audit";
  /** Type of action performed */
  action: AuditAction;
  /** Type of resource (e.g., "AgentRuntime", "PromptPack") */
  resourceType: string;
  /** Name of the specific resource (optional for list operations) */
  resourceName?: string;
  /** Workspace name */
  workspace: string;
  /** Kubernetes namespace */
  namespace: string;
  /** User identifier (email or username) */
  user: string;
  /** User's role in the workspace */
  role?: string;
  /** Auth provider (oauth, proxy, builtin, api-key) */
  authProvider?: string;
  /** Result of the operation */
  result: AuditResult;
  /** Error message if result is "error" or "denied" */
  errorMessage?: string;
  /** HTTP status code if applicable */
  statusCode?: number;
  /** Request path */
  path?: string;
  /** HTTP method */
  method?: string;
  /** Additional context-specific metadata */
  metadata?: Record<string, unknown>;
}

/**
 * Input for creating an audit log entry.
 * Timestamp and level are auto-generated.
 */
export type AuditInput = Omit<AuditEntry, "timestamp" | "level">;

/**
 * Check if audit logging is enabled.
 * Defaults to true in production, can be disabled via env var.
 */
export function isAuditLoggingEnabled(): boolean {
  const enabled = process.env.OMNIA_AUDIT_LOGGING_ENABLED;
  // Default to enabled unless explicitly disabled
  return enabled !== "false";
}

/**
 * Log an audit entry to stdout as JSON.
 *
 * @param input - Audit entry data (timestamp auto-generated)
 */
export function logAudit(input: AuditInput): void {
  if (!isAuditLoggingEnabled()) {
    return;
  }

  const entry: AuditEntry = {
    timestamp: new Date().toISOString(),
    level: "audit",
    ...input,
  };

  // Output as single-line JSON for log aggregation
  // eslint-disable-next-line no-console -- Audit logs use console.log for structured output to stdout
  console.log(JSON.stringify(entry));
}

/**
 * Create an audit logger bound to a specific context.
 * Useful for route handlers to avoid repeating common fields.
 *
 * @param context - Common fields for all audit entries
 * @returns A function that logs audit entries with the bound context
 */
export function createAuditLogger(
  context: Pick<AuditInput, "workspace" | "namespace" | "user" | "role" | "authProvider" | "path" | "method">
) {
  return function log(
    input: Omit<AuditInput, "workspace" | "namespace" | "user" | "role" | "authProvider" | "path" | "method">
  ): void {
    logAudit({
      ...context,
      ...input,
    });
  };
}

/**
 * Log a successful CRD operation.
 */
export function logCrdSuccess(
  action: AuditAction,
  resourceType: string,
  resourceName: string | undefined,
  workspace: string,
  namespace: string,
  user: string,
  role?: string,
  metadata?: Record<string, unknown>
): void {
  logAudit({
    action,
    resourceType,
    resourceName,
    workspace,
    namespace,
    user,
    role,
    result: "success",
    metadata,
  });
}

/**
 * Log a denied CRD operation (authorization failure).
 */
export function logCrdDenied(
  action: AuditAction,
  resourceType: string,
  resourceName: string | undefined,
  workspace: string,
  namespace: string,
  user: string,
  role?: string,
  errorMessage?: string
): void {
  logAudit({
    action,
    resourceType,
    resourceName,
    workspace,
    namespace,
    user,
    role,
    result: "denied",
    errorMessage,
    statusCode: 403,
  });
}

/**
 * Log a failed CRD operation (error).
 */
export function logCrdError(
  action: AuditAction,
  resourceType: string,
  resourceName: string | undefined,
  workspace: string,
  namespace: string,
  user: string,
  role?: string,
  errorMessage?: string,
  statusCode?: number
): void {
  logAudit({
    action,
    resourceType,
    resourceName,
    workspace,
    namespace,
    user,
    role,
    result: "error",
    errorMessage,
    statusCode,
  });
}

/**
 * Map HTTP method to audit action.
 */
export function methodToAction(method: string): AuditAction {
  switch (method.toUpperCase()) {
    case "GET":
      return "get";
    case "POST":
      return "create";
    case "PUT":
      return "update";
    case "PATCH":
      return "patch";
    case "DELETE":
      return "delete";
    default:
      return "get";
  }
}

/**
 * Log proxy usage for deprecation tracking.
 */
export function logProxyUsage(
  method: string,
  path: string,
  user?: string,
  userAgent?: string
): void {
  if (!isAuditLoggingEnabled()) {
    return;
  }

  const entry = {
    timestamp: new Date().toISOString(),
    level: "audit",
    action: "proxy_request",
    deprecated: true,
    method,
    path,
    user: user || "unknown",
    userAgent,
    message: "Deprecated operator proxy used. Migrate to workspace-scoped API routes.",
  };

  // eslint-disable-next-line no-console -- Audit logs use console.log for structured output to stdout
  console.log(JSON.stringify(entry));
}

/**
 * Structured warning log entry.
 */
interface WarnLogEntry {
  timestamp: string;
  level: "warn";
  message: string;
  context?: string;
  metadata?: Record<string, unknown>;
}

/**
 * Log a structured warning message.
 * Outputs JSON to stderr for log aggregation.
 */
export function logWarn(
  message: string,
  context?: string,
  metadata?: Record<string, unknown>
): void {
  const entry: WarnLogEntry = {
    timestamp: new Date().toISOString(),
    level: "warn",
    message,
    context,
    metadata,
  };

   
  console.warn(JSON.stringify(entry));
}

/**
 * Structured error log entry.
 */
interface ErrorLogEntry {
  timestamp: string;
  level: "error";
  message: string;
  context?: string;
  error?: string;
  stack?: string;
  metadata?: Record<string, unknown>;
}

/**
 * Extract error message from an unknown error value.
 */
function getErrorMessage(error: unknown): string | undefined {
  if (error instanceof Error) {
    return error.message;
  }
  if (error) {
    return String(error);
  }
  return undefined;
}

/**
 * Log a structured error message.
 * Outputs JSON to stderr for log aggregation.
 */
export function logError(
  message: string,
  error?: unknown,
  context?: string,
  metadata?: Record<string, unknown>
): void {
  const entry: ErrorLogEntry = {
    timestamp: new Date().toISOString(),
    level: "error",
    message,
    context,
    error: getErrorMessage(error),
    stack: error instanceof Error ? error.stack : undefined,
    metadata,
  };

  console.error(JSON.stringify(entry)); // NOSONAR - Structured logging to stderr
}
