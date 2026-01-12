import { describe, it, expect } from "vitest";
import {
  matchesMimePattern,
  isAllowedType,
  inferExtensionsFromMimeTypes,
  buildAttachmentConfig,
  buildAcceptString,
  formatFileSize,
  needsResize,
  calculateResizedDimensions,
  shouldCompress,
  getCompressionQuality,
  formatDuration,
  DEFAULT_ALLOWED_MIME_TYPES,
  DEFAULT_ALLOWED_EXTENSIONS,
  DEFAULT_MAX_FILE_SIZE,
  DEFAULT_MAX_FILES,
} from "./attachment-utils";

describe("attachment-utils", () => {
  describe("matchesMimePattern", () => {
    it("should match exact MIME types", () => {
      expect(matchesMimePattern("image/png", "image/png")).toBe(true);
      expect(matchesMimePattern("image/png", "image/jpeg")).toBe(false);
    });

    it("should match wildcard patterns", () => {
      expect(matchesMimePattern("image/png", "image/*")).toBe(true);
      expect(matchesMimePattern("image/jpeg", "image/*")).toBe(true);
      expect(matchesMimePattern("audio/mp3", "image/*")).toBe(false);
    });

    it("should match universal wildcard", () => {
      expect(matchesMimePattern("image/png", "*/*")).toBe(true);
      expect(matchesMimePattern("audio/mp3", "*/*")).toBe(true);
      expect(matchesMimePattern("application/pdf", "*/*")).toBe(true);
    });

    it("should handle empty or unusual patterns", () => {
      expect(matchesMimePattern("image/png", "")).toBe(false);
      expect(matchesMimePattern("", "image/*")).toBe(false);
    });
  });

  describe("isAllowedType", () => {
    const allowedMimeTypes = ["image/*", "application/pdf"];
    const allowedExtensions = [".png", ".jpg", ".pdf", ".doc"];

    it("should allow files matching MIME type patterns", () => {
      const result = isAllowedType(
        { type: "image/png", name: "photo.png" },
        allowedMimeTypes,
        allowedExtensions
      );
      expect(result.allowed).toBe(true);
    });

    it("should allow files matching exact MIME types", () => {
      const result = isAllowedType(
        { type: "application/pdf", name: "document.pdf" },
        allowedMimeTypes,
        allowedExtensions
      );
      expect(result.allowed).toBe(true);
    });

    it("should allow files by extension when MIME type is generic", () => {
      const result = isAllowedType(
        { type: "application/octet-stream", name: "document.doc" },
        allowedMimeTypes,
        allowedExtensions
      );
      expect(result.allowed).toBe(true);
    });

    it("should reject files not matching any pattern", () => {
      const result = isAllowedType(
        { type: "video/mp4", name: "movie.mp4" },
        allowedMimeTypes,
        allowedExtensions
      );
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("not allowed");
    });

    it("should handle files without extensions", () => {
      const result = isAllowedType(
        { type: "text/plain", name: "README" },
        ["text/plain"],
        []
      );
      expect(result.allowed).toBe(true);
    });
  });

  describe("inferExtensionsFromMimeTypes", () => {
    it("should infer extensions from wildcard patterns", () => {
      const extensions = inferExtensionsFromMimeTypes(["image/*"]);
      expect(extensions).toContain(".png");
      expect(extensions).toContain(".jpg");
      expect(extensions).toContain(".jpeg");
      expect(extensions).toContain(".gif");
    });

    it("should infer extensions from specific MIME types", () => {
      const extensions = inferExtensionsFromMimeTypes(["application/pdf"]);
      expect(extensions).toContain(".pdf");
    });

    it("should combine extensions from multiple patterns", () => {
      const extensions = inferExtensionsFromMimeTypes([
        "image/png",
        "audio/mpeg",
      ]);
      expect(extensions).toContain(".png");
      expect(extensions).toContain(".mp3");
    });

    it("should return empty array for unknown MIME types", () => {
      const extensions = inferExtensionsFromMimeTypes(["application/unknown"]);
      expect(extensions).toEqual([]);
    });

    it("should deduplicate extensions", () => {
      const extensions = inferExtensionsFromMimeTypes([
        "image/png",
        "image/*", // Also includes .png
      ]);
      const pngCount = extensions.filter((e) => e === ".png").length;
      expect(pngCount).toBe(1);
    });
  });

  describe("buildAcceptString", () => {
    it("should combine MIME types and extensions", () => {
      const result = buildAcceptString(["image/png"], [".png"]);
      expect(result).toBe("image/png,.png");
    });

    it("should handle empty arrays", () => {
      expect(buildAcceptString([], [])).toBe("");
    });

    it("should handle multiple values", () => {
      const result = buildAcceptString(
        ["image/png", "image/jpeg"],
        [".png", ".jpg"]
      );
      expect(result).toBe("image/png,image/jpeg,.png,.jpg");
    });
  });

  describe("formatFileSize", () => {
    it("should format bytes", () => {
      expect(formatFileSize(500)).toBe("500 B");
      expect(formatFileSize(0)).toBe("0 B");
    });

    it("should format kilobytes", () => {
      expect(formatFileSize(1024)).toBe("1.0 KB");
      expect(formatFileSize(1536)).toBe("1.5 KB");
    });

    it("should format megabytes", () => {
      expect(formatFileSize(1024 * 1024)).toBe("1.0 MB");
      expect(formatFileSize(10 * 1024 * 1024)).toBe("10.0 MB");
    });
  });

  describe("buildAttachmentConfig", () => {
    it("should return defaults when no config provided", () => {
      const config = buildAttachmentConfig();

      expect(config.allowedMimeTypes).toEqual(DEFAULT_ALLOWED_MIME_TYPES);
      expect(config.maxFileSize).toBe(DEFAULT_MAX_FILE_SIZE);
      expect(config.maxFiles).toBe(DEFAULT_MAX_FILES);
    });

    it("should return defaults when undefined config provided", () => {
      const config = buildAttachmentConfig(undefined);

      expect(config.allowedMimeTypes).toEqual(DEFAULT_ALLOWED_MIME_TYPES);
      expect(config.maxFileSize).toBe(DEFAULT_MAX_FILE_SIZE);
      expect(config.maxFiles).toBe(DEFAULT_MAX_FILES);
    });

    it("should use custom MIME types when provided", () => {
      const customTypes = ["image/*", "application/pdf"];
      const config = buildAttachmentConfig({
        allowedAttachmentTypes: customTypes,
      });

      expect(config.allowedMimeTypes).toEqual(customTypes);
    });

    it("should infer extensions when not explicitly provided", () => {
      const config = buildAttachmentConfig({
        allowedAttachmentTypes: ["image/png"],
      });

      expect(config.allowedExtensions).toContain(".png");
    });

    it("should use custom extensions when provided", () => {
      const customExtensions = [".custom"];
      const config = buildAttachmentConfig({
        allowedExtensions: customExtensions,
      });

      expect(config.allowedExtensions).toEqual(customExtensions);
    });

    it("should use custom maxFileSize when provided", () => {
      const config = buildAttachmentConfig({
        maxFileSize: 5 * 1024 * 1024,
      });

      expect(config.maxFileSize).toBe(5 * 1024 * 1024);
    });

    it("should use custom maxFiles when provided", () => {
      const config = buildAttachmentConfig({
        maxFiles: 10,
      });

      expect(config.maxFiles).toBe(10);
    });

    it("should build correct acceptString", () => {
      const config = buildAttachmentConfig({
        allowedAttachmentTypes: ["image/png"],
        allowedExtensions: [".png"],
      });

      expect(config.acceptString).toBe("image/png,.png");
    });
  });

  describe("default constants", () => {
    it("should have sensible default MIME types", () => {
      expect(DEFAULT_ALLOWED_MIME_TYPES).toContain("image/png");
      expect(DEFAULT_ALLOWED_MIME_TYPES).toContain("application/pdf");
      expect(DEFAULT_ALLOWED_MIME_TYPES).toContain("text/plain");
    });

    it("should have sensible default extensions", () => {
      expect(DEFAULT_ALLOWED_EXTENSIONS).toContain(".png");
      expect(DEFAULT_ALLOWED_EXTENSIONS).toContain(".pdf");
      expect(DEFAULT_ALLOWED_EXTENSIONS).toContain(".txt");
    });

    it("should have 10MB as default max file size", () => {
      expect(DEFAULT_MAX_FILE_SIZE).toBe(10 * 1024 * 1024);
    });

    it("should have 5 as default max files", () => {
      expect(DEFAULT_MAX_FILES).toBe(5);
    });
  });

  describe("needsResize", () => {
    it("should return false when no maxDimensions provided", () => {
      expect(needsResize(1000, 1000, undefined)).toBe(false);
    });

    it("should return false when image fits within max dimensions", () => {
      expect(needsResize(500, 500, { width: 1000, height: 1000 })).toBe(false);
      expect(needsResize(1000, 500, { width: 1000, height: 1000 })).toBe(false);
      expect(needsResize(500, 1000, { width: 1000, height: 1000 })).toBe(false);
    });

    it("should return true when image exceeds max dimensions", () => {
      expect(needsResize(1500, 500, { width: 1000, height: 1000 })).toBe(true);
      expect(needsResize(500, 1500, { width: 1000, height: 1000 })).toBe(true);
      expect(needsResize(1500, 1500, { width: 1000, height: 1000 })).toBe(true);
    });
  });

  describe("calculateResizedDimensions", () => {
    it("should maintain aspect ratio when scaling down wide images", () => {
      const result = calculateResizedDimensions(2000, 1000, { width: 1000, height: 1000 });
      expect(result.width).toBe(1000);
      expect(result.height).toBe(500);
    });

    it("should maintain aspect ratio when scaling down tall images", () => {
      const result = calculateResizedDimensions(1000, 2000, { width: 1000, height: 1000 });
      expect(result.width).toBe(500);
      expect(result.height).toBe(1000);
    });

    it("should not scale up images smaller than max", () => {
      const result = calculateResizedDimensions(500, 500, { width: 1000, height: 1000 });
      expect(result.width).toBe(500);
      expect(result.height).toBe(500);
    });

    it("should handle square images", () => {
      const result = calculateResizedDimensions(2000, 2000, { width: 1000, height: 1000 });
      expect(result.width).toBe(1000);
      expect(result.height).toBe(1000);
    });
  });

  describe("shouldCompress", () => {
    it("should return true when file exceeds max size", () => {
      expect(shouldCompress(15 * 1024 * 1024, 10 * 1024 * 1024, undefined)).toBe(true);
    });

    it("should return false when file is within max size and no guidance", () => {
      expect(shouldCompress(5 * 1024 * 1024, 10 * 1024 * 1024, undefined)).toBe(false);
    });

    it("should return false when guidance is none", () => {
      expect(shouldCompress(5 * 1024 * 1024, 10 * 1024 * 1024, "none")).toBe(false);
    });

    it("should return true for any compression guidance except none", () => {
      expect(shouldCompress(5 * 1024 * 1024, 10 * 1024 * 1024, "lossless")).toBe(true);
      expect(shouldCompress(5 * 1024 * 1024, 10 * 1024 * 1024, "lossy-high")).toBe(true);
      expect(shouldCompress(5 * 1024 * 1024, 10 * 1024 * 1024, "lossy-medium")).toBe(true);
      expect(shouldCompress(5 * 1024 * 1024, 10 * 1024 * 1024, "lossy-low")).toBe(true);
    });

    it("should handle undefined max size", () => {
      expect(shouldCompress(5 * 1024 * 1024, undefined, "lossy-medium")).toBe(true);
      expect(shouldCompress(5 * 1024 * 1024, undefined, undefined)).toBe(false);
    });
  });

  describe("getCompressionQuality", () => {
    it("should return 1.0 for lossless", () => {
      expect(getCompressionQuality("lossless")).toBe(1.0);
    });

    it("should return 0.92 for lossy-high", () => {
      expect(getCompressionQuality("lossy-high")).toBe(0.92);
    });

    it("should return 0.85 for lossy-medium", () => {
      expect(getCompressionQuality("lossy-medium")).toBe(0.85);
    });

    it("should return 0.7 for lossy-low", () => {
      expect(getCompressionQuality("lossy-low")).toBe(0.7);
    });

    it("should return default 0.92 for undefined", () => {
      expect(getCompressionQuality(undefined)).toBe(0.92);
    });

    it("should return default 0.92 for none", () => {
      expect(getCompressionQuality("none")).toBe(0.92);
    });
  });

  describe("formatDuration", () => {
    it("should format seconds only", () => {
      expect(formatDuration(30)).toBe("30s");
      expect(formatDuration(59)).toBe("59s");
    });

    it("should format minutes and seconds", () => {
      expect(formatDuration(60)).toBe("1m 0s");
      expect(formatDuration(90)).toBe("1m 30s");
      expect(formatDuration(3599)).toBe("59m 59s");
    });

    it("should format hours and minutes", () => {
      expect(formatDuration(3600)).toBe("1h 0m");
      expect(formatDuration(3660)).toBe("1h 1m");
      expect(formatDuration(7200)).toBe("2h 0m");
    });
  });
});
