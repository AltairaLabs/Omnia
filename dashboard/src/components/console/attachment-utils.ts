/**
 * Utilities for file attachment validation and configuration.
 * Supports configurable MIME types with wildcard patterns (e.g., "image/*").
 */

import type {
  ConsoleConfig,
  Dimensions,
  CompressionGuidance,
} from "@/types/agent-runtime";

// Default values matching current hardcoded behavior
export const DEFAULT_ALLOWED_MIME_TYPES = [
  // Images
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
  // Audio
  "audio/mpeg",
  "audio/wav",
  "audio/ogg",
  // Documents
  "application/pdf",
  "text/plain",
  "text/markdown",
  // Code files (browsers may report various MIME types)
  "text/javascript",
  "text/typescript",
  "application/javascript",
  "application/typescript",
  "text/x-python",
  "application/x-python-code",
  // Data
  "text/csv",
  "application/json",
];

export const DEFAULT_ALLOWED_EXTENSIONS = [
  ".png", ".jpg", ".jpeg", ".gif", ".webp",  // Images
  ".mp3", ".wav", ".ogg",                     // Audio
  ".pdf", ".txt", ".md",                      // Documents
  ".js", ".ts", ".jsx", ".tsx", ".py",        // Code
  ".csv", ".json",                            // Data
];

export const DEFAULT_MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB
export const DEFAULT_MAX_FILES = 5;

export interface AttachmentConfig {
  allowedMimeTypes: string[];
  allowedExtensions: string[];
  maxFileSize: number;
  maxFiles: number;
  acceptString: string;
}

/**
 * Build accept string for file input from MIME types and extensions.
 */
export function buildAcceptString(mimeTypes: string[], extensions: string[]): string {
  return [...mimeTypes, ...extensions].join(",");
}

/**
 * Check if a MIME type matches a pattern (supports wildcards like "image/*").
 */
export function matchesMimePattern(type: string, pattern: string): boolean {
  if (pattern === "*/*") return true;
  if (pattern.endsWith("/*")) {
    const category = pattern.slice(0, -2);
    return type.startsWith(category + "/");
  }
  return type === pattern;
}

/**
 * Validate file type against allowed types.
 * Returns an object with `allowed` boolean and optional `reason` string.
 */
export function isAllowedType(
  file: { type: string; name: string },
  allowedMimeTypes: string[],
  allowedExtensions: string[]
): { allowed: boolean; reason?: string } {
  // Check MIME type patterns
  for (const pattern of allowedMimeTypes) {
    if (matchesMimePattern(file.type, pattern)) {
      return { allowed: true };
    }
  }

  // Fallback to extension check for files with generic MIME types
  const ext = "." + file.name.split(".").pop()?.toLowerCase();
  if (allowedExtensions.includes(ext)) {
    return { allowed: true };
  }

  return {
    allowed: false,
    reason: `File type "${file.type || ext}" is not allowed`,
  };
}

/**
 * Map of MIME patterns to common file extensions.
 * Used to infer extensions when only MIME types are configured.
 */
const MIME_TO_EXTENSIONS: Record<string, string[]> = {
  // Wildcards
  "image/*": [".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp"],
  "audio/*": [".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac"],
  "video/*": [".mp4", ".webm", ".mov", ".avi", ".mkv"],
  "text/*": [".txt", ".md", ".csv", ".json", ".js", ".ts", ".py", ".html", ".css"],
  // Specific types
  "image/png": [".png"],
  "image/jpeg": [".jpg", ".jpeg"],
  "image/gif": [".gif"],
  "image/webp": [".webp"],
  "image/svg+xml": [".svg"],
  "audio/mpeg": [".mp3"],
  "audio/wav": [".wav"],
  "audio/ogg": [".ogg"],
  "video/mp4": [".mp4"],
  "video/webm": [".webm"],
  "application/pdf": [".pdf"],
  "text/plain": [".txt"],
  "text/markdown": [".md"],
  "text/csv": [".csv"],
  "application/json": [".json"],
  "text/javascript": [".js"],
  "application/javascript": [".js"],
  "text/typescript": [".ts"],
  "application/typescript": [".ts"],
  "text/x-python": [".py"],
  "application/x-python-code": [".py"],
};

/**
 * Infer file extensions from MIME types (for wildcards and specific types).
 */
export function inferExtensionsFromMimeTypes(mimeTypes: string[]): string[] {
  const extensions = new Set<string>();

  for (const type of mimeTypes) {
    const exts = MIME_TO_EXTENSIONS[type];
    if (exts) {
      exts.forEach((e) => extensions.add(e));
    }
  }

  return Array.from(extensions);
}

/**
 * Build attachment config from agent console config.
 * Falls back to defaults when config is not provided.
 */
export function buildAttachmentConfig(consoleConfig?: ConsoleConfig): AttachmentConfig {
  const allowedMimeTypes = consoleConfig?.allowedAttachmentTypes ?? DEFAULT_ALLOWED_MIME_TYPES;
  const allowedExtensions =
    consoleConfig?.allowedExtensions ?? inferExtensionsFromMimeTypes(allowedMimeTypes);
  const maxFileSize = consoleConfig?.maxFileSize ?? DEFAULT_MAX_FILE_SIZE;
  const maxFiles = consoleConfig?.maxFiles ?? DEFAULT_MAX_FILES;

  return {
    allowedMimeTypes,
    allowedExtensions,
    maxFileSize,
    maxFiles,
    acceptString: buildAcceptString(allowedMimeTypes, allowedExtensions),
  };
}

/**
 * Format file size for display.
 */
export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// =============================================================================
// Media Requirements Utilities
// =============================================================================

/**
 * Check if an image needs resizing based on maximum dimensions.
 */
export function needsResize(
  actualWidth: number,
  actualHeight: number,
  maxDimensions?: Dimensions
): boolean {
  if (!maxDimensions) return false;
  return actualWidth > maxDimensions.width || actualHeight > maxDimensions.height;
}

/**
 * Calculate target dimensions while maintaining aspect ratio.
 * Returns dimensions that fit within maxDimensions while preserving aspect ratio.
 */
export function calculateResizedDimensions(
  actualWidth: number,
  actualHeight: number,
  maxDimensions: Dimensions
): Dimensions {
  const widthRatio = maxDimensions.width / actualWidth;
  const heightRatio = maxDimensions.height / actualHeight;
  const ratio = Math.min(widthRatio, heightRatio, 1);

  return {
    width: Math.round(actualWidth * ratio),
    height: Math.round(actualHeight * ratio),
  };
}

/**
 * Determine if image compression is recommended based on file size and guidance.
 */
export function shouldCompress(
  fileSize: number,
  maxSizeBytes: number | undefined,
  guidance: CompressionGuidance | undefined
): boolean {
  // Always compress if file exceeds max size
  if (maxSizeBytes && fileSize > maxSizeBytes) return true;
  // Compress based on guidance (anything except "none" suggests compression)
  if (guidance && guidance !== "none") return true;
  return false;
}

/**
 * Get recommended compression quality based on guidance.
 * Returns a value from 0-1 suitable for canvas.toBlob quality parameter.
 */
export function getCompressionQuality(guidance: CompressionGuidance | undefined): number {
  switch (guidance) {
    case "lossless":
      return 1;
    case "lossy-high":
      return 0.92;
    case "lossy-medium":
      return 0.85;
    case "lossy-low":
      return 0.7;
    default:
      return 0.92; // Default to high quality
  }
}

/**
 * Format duration in seconds for display.
 */
export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${mins}m`;
}

// =============================================================================
// Base64 Encoding Utilities
// =============================================================================

/**
 * Base64 encoding adds overhead: 4 output bytes for every 3 input bytes.
 * This results in approximately 33% size increase.
 */
export const BASE64_OVERHEAD_FACTOR = 4 / 3; // ~1.333

/**
 * Calculate the effective maximum file size for upload, accounting for base64 overhead.
 *
 * When files are sent as data URLs (base64 encoded), they grow by ~33%.
 * This function calculates the maximum raw file size that will stay within
 * a given limit after base64 encoding.
 *
 * @param encodedLimit - The maximum allowed size after base64 encoding (e.g., gRPC limit)
 * @returns The maximum raw file size that will fit within the limit after encoding
 *
 * @example
 * // With a 16MB gRPC limit, maximum raw file size is ~12MB
 * getEffectiveMaxSize(16 * 1024 * 1024) // Returns ~11,930,464 bytes
 */
export function getEffectiveMaxSize(encodedLimit: number): number {
  return Math.floor(encodedLimit / BASE64_OVERHEAD_FACTOR);
}

/**
 * Calculate the encoded size of a file after base64 conversion.
 *
 * @param rawSize - The raw file size in bytes
 * @returns The approximate size after base64 encoding
 */
export function getEncodedSize(rawSize: number): number {
  return Math.ceil(rawSize * BASE64_OVERHEAD_FACTOR);
}
