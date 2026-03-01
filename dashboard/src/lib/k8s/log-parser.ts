/**
 * Structured log parser for Kubernetes pod logs.
 *
 * Handles three log formats:
 * 1. Zap production JSON: K8s-timestamp + JSON body with level/msg/ts fields
 * 2. Zap development: K8s-timestamp + tab-separated LEVEL/CALLER/MSG/JSON
 * 3. Unstructured text: fallback with K8s timestamp and raw message
 */

import type { LogEntry } from "@/lib/data/types";

/** Keys excluded from the structured fields output */
const EXCLUDED_KEYS = new Set(["level", "msg", "ts", "time", "caller", "stacktrace", "logger"]);

/** Map Zap level strings to normalized level values */
const ZAP_LEVEL_MAP: Record<string, string> = {
  debug: "debug",
  info: "info",
  warn: "warn",
  warning: "warn",
  error: "error",
  dpanic: "error",
  panic: "error",
  fatal: "error",
};

/**
 * Strip the Kubernetes-injected timestamp prefix from a log line.
 * Returns [k8sTimestamp, body] or [null, originalLine] if no prefix found.
 */
function stripK8sTimestamp(line: string): [string | null, string] {
  const spaceIndex = line.indexOf(" ");
  if (spaceIndex > 0) {
    const prefix = line.substring(0, spaceIndex);
    if (/^\d{4}-\d{2}-\d{2}T/.test(prefix)) {
      return [prefix, line.substring(spaceIndex + 1)];
    }
  }
  return [null, line];
}

/**
 * Convert a Zap `ts` field (Unix epoch float) to an ISO timestamp string.
 */
function zapTsToISO(ts: unknown): string | null {
  if (typeof ts === "number" && ts > 0) {
    return new Date(ts * 1000).toISOString();
  }
  if (typeof ts === "string") {
    const parsed = new Date(ts);
    if (!Number.isNaN(parsed.getTime())) {
      return parsed.toISOString();
    }
  }
  return null;
}

/**
 * Extract remaining fields from a parsed JSON object, excluding known metadata keys.
 */
function extractFields(obj: Record<string, unknown>): Record<string, unknown> | undefined {
  const fields: Record<string, unknown> = {};
  let count = 0;
  for (const [key, value] of Object.entries(obj)) {
    if (!EXCLUDED_KEYS.has(key)) {
      fields[key] = value;
      count++;
    }
  }
  return count > 0 ? fields : undefined;
}

/**
 * Try to parse the body as Zap production JSON.
 * Expected shape: {"level":"info","ts":1706789012.123,"msg":"...","caller":"...", ...}
 */
function tryParseZapJSON(
  body: string,
  k8sTimestamp: string | null,
  containerName: string
): LogEntry | null {
  if (!body.startsWith("{")) {
    return null;
  }

  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(body);
  } catch {
    return null;
  }

  if (typeof parsed !== "object" || parsed === null) {
    return null;
  }

  const msg = typeof parsed.msg === "string" ? parsed.msg : body;
  const rawLevel = typeof parsed.level === "string" ? parsed.level.toLowerCase() : "";
  const level = ZAP_LEVEL_MAP[rawLevel] || "";
  const appTimestamp = zapTsToISO(parsed.ts) ?? zapTsToISO(parsed.time);
  const timestamp = appTimestamp ?? k8sTimestamp ?? new Date().toISOString();

  return {
    timestamp,
    level,
    message: msg,
    container: containerName,
    fields: extractFields(parsed),
  };
}

/**
 * Try to parse the body as Zap development format.
 * Pattern: TIMESTAMP\tLEVEL\tCALLER\tMSG[\tJSON_FIELDS]
 * The timestamp here is the app timestamp (not K8s prefix).
 */
function tryParseZapDev(
  body: string,
  k8sTimestamp: string | null,
  containerName: string
): LogEntry | null {
  const parts = body.split("\t");
  if (parts.length < 4) {
    return null;
  }

  // parts[0] = app timestamp, parts[1] = LEVEL, parts[2] = caller/logger, parts[3] = message
  const appTs = parts[0];
  const rawLevel = parts[1].toLowerCase();
  const level = ZAP_LEVEL_MAP[rawLevel];

  if (!level) {
    return null;
  }

  const msg = parts[3];

  // Try to use the app timestamp
  let timestamp = k8sTimestamp ?? new Date().toISOString();
  const parsedAppTs = new Date(appTs);
  if (!Number.isNaN(parsedAppTs.getTime())) {
    timestamp = parsedAppTs.toISOString();
  }

  // parts[4+] may be JSON fields
  let fields: Record<string, unknown> | undefined;
  if (parts.length > 4) {
    const jsonPart = parts.slice(4).join("\t");
    try {
      const parsed = JSON.parse(jsonPart);
      if (typeof parsed === "object" && parsed !== null) {
        fields = parsed as Record<string, unknown>;
      }
    } catch {
      // Not valid JSON — ignore
    }
  }

  return {
    timestamp,
    level,
    message: msg,
    container: containerName,
    fields,
  };
}

/**
 * Parse a single log line into a structured LogEntry.
 *
 * Handles Zap production JSON, Zap development format, and unstructured text.
 * The K8s timestamp prefix is stripped and the application's own timestamp is
 * used when available.
 */
export function parseLogLine(line: string, containerName: string): LogEntry {
  const [k8sTimestamp, body] = stripK8sTimestamp(line);

  // Try Zap production JSON first
  const zapJson = tryParseZapJSON(body, k8sTimestamp, containerName);
  if (zapJson) {
    return zapJson;
  }

  // Try Zap development format
  const zapDev = tryParseZapDev(body, k8sTimestamp, containerName);
  if (zapDev) {
    return zapDev;
  }

  // Fallback: unstructured text
  return {
    timestamp: k8sTimestamp ?? new Date().toISOString(),
    level: "",
    message: body,
    container: containerName,
  };
}
