/**
 * Client-side image processing utilities.
 *
 * Uses the Canvas API to resize and compress images before upload.
 * This reduces bandwidth usage and ensures images fit within model constraints.
 */

import type { Dimensions } from "@/types/agent-runtime";

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

