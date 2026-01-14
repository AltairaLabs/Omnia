/**
 * Client-side image processing utilities.
 *
 * Uses the Canvas API to resize and compress images before upload.
 * This reduces bandwidth usage and ensures images fit within model constraints.
 */

import {
  needsResize,
  calculateResizedDimensions,
  getCompressionQuality,
} from "@/components/console/attachment-utils";
import type { Dimensions, CompressionGuidance } from "@/types/agent-runtime";

export type ImageFormat = "image/jpeg" | "image/png" | "image/webp";

export interface ProcessImageOptions {
  /** Maximum dimensions for the output image */
  maxDimensions: Dimensions;
  /** Output image format */
  format?: ImageFormat;
  /** Compression quality (0-1, only applies to JPEG/WebP) */
  quality?: number;
  /** Compression guidance from media requirements */
  compressionGuidance?: CompressionGuidance;
}

export interface ProcessedImage {
  /** The processed image as a Blob */
  blob: Blob;
  /** The final dimensions after processing */
  dimensions: Dimensions;
  /** The original dimensions before processing */
  originalDimensions: Dimensions;
  /** Whether the image was resized */
  wasResized: boolean;
  /** The size reduction in bytes (original - processed) */
  sizeReduction: number;
}

/**
 * Load an image from a File and return its dimensions.
 */
export function getImageDimensions(file: File): Promise<Dimensions> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    const url = URL.createObjectURL(file);

    img.onload = () => {
      URL.revokeObjectURL(url);
      resolve({ width: img.naturalWidth, height: img.naturalHeight });
    };

    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`Failed to load image: ${file.name}`));
    };

    img.src = url;
  });
}

/**
 * Process an image file: resize if needed and compress.
 *
 * This function:
 * 1. Loads the image into memory
 * 2. Checks if it needs resizing based on maxDimensions
 * 3. Draws it to a canvas at the target size
 * 4. Exports as the specified format with compression
 *
 * @param file - The image file to process
 * @param options - Processing options (max dimensions, format, quality)
 * @returns Promise resolving to the processed image data
 */
export async function processImage(
  file: File,
  options: ProcessImageOptions
): Promise<ProcessedImage> {
  const { maxDimensions, compressionGuidance } = options;

  // Determine format and quality
  const format = options.format ?? inferOutputFormat(file.type);
  const quality = options.quality ?? getCompressionQuality(compressionGuidance);

  return new Promise((resolve, reject) => {
    const img = new Image();
    const url = URL.createObjectURL(file);

    img.onload = () => {
      URL.revokeObjectURL(url);

      const originalDimensions: Dimensions = {
        width: img.naturalWidth,
        height: img.naturalHeight,
      };

      // Calculate target dimensions
      const wasResized = needsResize(
        originalDimensions.width,
        originalDimensions.height,
        maxDimensions
      );

      const targetDimensions = wasResized
        ? calculateResizedDimensions(
            originalDimensions.width,
            originalDimensions.height,
            maxDimensions
          )
        : originalDimensions;

      // Create canvas and draw image
      const canvas = document.createElement("canvas");
      canvas.width = targetDimensions.width;
      canvas.height = targetDimensions.height;

      const ctx = canvas.getContext("2d");
      if (!ctx) {
        reject(new Error("Failed to get canvas 2D context"));
        return;
      }

      // Use high-quality image smoothing for resize
      ctx.imageSmoothingEnabled = true;
      ctx.imageSmoothingQuality = "high";

      // Draw the image scaled to target dimensions
      ctx.drawImage(img, 0, 0, targetDimensions.width, targetDimensions.height);

      // Convert to blob
      canvas.toBlob(
        (blob) => {
          if (!blob) {
            reject(new Error("Failed to create image blob"));
            return;
          }

          resolve({
            blob,
            dimensions: targetDimensions,
            originalDimensions,
            wasResized,
            sizeReduction: file.size - blob.size,
          });
        },
        format,
        quality
      );
    };

    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`Failed to load image: ${file.name}`));
    };

    img.src = url;
  });
}

/**
 * Infer the best output format based on input type.
 *
 * - PNGs with transparency → keep as PNG
 * - Most images → JPEG for better compression
 * - WebP input → keep as WebP
 */
function inferOutputFormat(inputType: string): ImageFormat {
  // Keep WebP as WebP (good compression, widely supported)
  if (inputType === "image/webp") {
    return "image/webp";
  }

  // Keep PNG as PNG to preserve transparency
  // (could add transparency detection for optimization)
  if (inputType === "image/png") {
    return "image/png";
  }

  // Default to JPEG for good compression
  return "image/jpeg";
}

/**
 * Convert a Blob to a data URL for display/upload.
 */
export function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error("Failed to convert blob to data URL"));
    reader.readAsDataURL(blob);
  });
}

/**
 * Create a File object from a processed blob.
 */
export function createProcessedFile(
  blob: Blob,
  originalName: string,
  format: ImageFormat
): File {
  // Generate new filename with correct extension
  const baseName = originalName.replace(/\.[^.]+$/, "");
  const extension = format.split("/")[1];
  const newName = `${baseName}.${extension}`;

  return new File([blob], newName, { type: format });
}
