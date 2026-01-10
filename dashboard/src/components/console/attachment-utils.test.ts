import { describe, it, expect } from "vitest";
import {
  matchesMimePattern,
  isAllowedType,
  inferExtensionsFromMimeTypes,
  buildAttachmentConfig,
  buildAcceptString,
  formatFileSize,
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
});
