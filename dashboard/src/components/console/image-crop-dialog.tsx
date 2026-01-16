"use client";

/**
 * Image crop dialog for user-controlled image selection.
 *
 * Allows users to select a region of an image before uploading.
 * The cropped region is then resized to fit within model constraints.
 */

import { useState, useRef, useCallback, useEffect } from "react";
import ReactCrop, { type Crop, type PixelCrop, centerCrop, makeAspectCrop } from "react-image-crop";
import "react-image-crop/dist/ReactCrop.css";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { calculateResizedDimensions, getCompressionQuality } from "./attachment-utils";
import type { Dimensions, CompressionGuidance } from "@/types/agent-runtime";

export interface ImageCropDialogProps {
  /** The file to crop */
  file: File;
  /** Maximum output dimensions */
  maxDimensions: Dimensions;
  /** Preferred output format */
  preferredFormat?: "image/jpeg" | "image/png" | "image/webp";
  /** Compression guidance */
  compressionGuidance?: CompressionGuidance;
  /** Called when crop is complete */
  onComplete: (result: { blob: Blob; file: File }) => void;
  /** Called when dialog is cancelled */
  onCancel: () => void;
  /** Whether the dialog is open */
  open: boolean;
}

/**
 * Center a crop selection on an image.
 */
function centerAspectCrop(
  mediaWidth: number,
  mediaHeight: number,
  aspect: number
): Crop {
  return centerCrop(
    makeAspectCrop(
      { unit: "%", width: 90 },
      aspect,
      mediaWidth,
      mediaHeight
    ),
    mediaWidth,
    mediaHeight
  );
}

interface CanvasProcessingParams {
  img: HTMLImageElement;
  sourceRect: { x: number; y: number; width: number; height: number };
  maxDimensions: Dimensions;
  preferredFormat: "image/jpeg" | "image/png" | "image/webp";
  compressionGuidance: CompressionGuidance | undefined;
  originalFileName: string;
}

/**
 * Process an image region and create a new file.
 * Shared logic for both crop and skip operations.
 */
async function processImageToFile({
  img,
  sourceRect,
  maxDimensions,
  preferredFormat,
  compressionGuidance,
  originalFileName,
}: CanvasProcessingParams): Promise<{ blob: Blob; file: File }> {
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Failed to get canvas context");

  // Calculate final dimensions (fit within max, maintain aspect)
  const finalDimensions = calculateResizedDimensions(
    sourceRect.width,
    sourceRect.height,
    maxDimensions
  );

  canvas.width = finalDimensions.width;
  canvas.height = finalDimensions.height;

  // Use high-quality scaling
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";

  // Draw the source region to canvas
  ctx.drawImage(
    img,
    sourceRect.x,
    sourceRect.y,
    sourceRect.width,
    sourceRect.height,
    0,
    0,
    finalDimensions.width,
    finalDimensions.height
  );

  // Convert to blob
  const quality = getCompressionQuality(compressionGuidance);
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("Failed to create blob"))),
      preferredFormat,
      quality
    );
  });

  // Create a new File with the correct name and type
  const extension = preferredFormat.split("/")[1];
  const baseName = originalFileName.replace(/\.[^.]+$/, "");
  const newFile = new File([blob], `${baseName}.${extension}`, {
    type: preferredFormat,
  });

  return { blob, file: newFile };
}

export function ImageCropDialog({
  file,
  maxDimensions,
  preferredFormat = "image/jpeg",
  compressionGuidance,
  onComplete,
  onCancel,
  open,
}: Readonly<ImageCropDialogProps>) {
  const [crop, setCrop] = useState<Crop>();
  const [completedCrop, setCompletedCrop] = useState<PixelCrop>();
  const [imageSrc, setImageSrc] = useState<string>("");
  const [isProcessing, setIsProcessing] = useState(false);
  const imgRef = useRef<HTMLImageElement>(null);

  // Load image when file changes
  useEffect(() => {
    if (!file || !open) return;

    const url = URL.createObjectURL(file);
    setImageSrc(url);

    return () => {
      URL.revokeObjectURL(url);
      setImageSrc("");
      setCrop(undefined);
      setCompletedCrop(undefined);
    };
  }, [file, open]);

  // Initialize crop when image loads
  const onImageLoad = useCallback(
    (e: React.SyntheticEvent<HTMLImageElement>) => {
      const { naturalWidth: width, naturalHeight: height } = e.currentTarget;

      // Calculate aspect ratio to match max dimensions
      const aspect = maxDimensions.width / maxDimensions.height;
      const initialCrop = centerAspectCrop(width, height, aspect);
      setCrop(initialCrop);
    },
    [maxDimensions]
  );

  // Process the crop and create output blob
  const handleApply = useCallback(async () => {
    if (!imgRef.current || !completedCrop) return;

    setIsProcessing(true);

    try {
      const img = imgRef.current;
      const scaleX = img.naturalWidth / img.width;
      const scaleY = img.naturalHeight / img.height;

      // Calculate the actual crop region in pixels
      const result = await processImageToFile({
        img,
        sourceRect: {
          x: completedCrop.x * scaleX,
          y: completedCrop.y * scaleY,
          width: completedCrop.width * scaleX,
          height: completedCrop.height * scaleY,
        },
        maxDimensions,
        preferredFormat,
        compressionGuidance,
        originalFileName: file.name,
      });

      onComplete(result);
    } catch (error) {
      console.error("Failed to process crop:", error);
    } finally {
      setIsProcessing(false);
    }
  }, [completedCrop, file, maxDimensions, preferredFormat, compressionGuidance, onComplete]);

  // Skip crop and auto-resize the whole image
  const handleSkip = useCallback(async () => {
    if (!imgRef.current) return;

    setIsProcessing(true);

    try {
      const img = imgRef.current;

      const result = await processImageToFile({
        img,
        sourceRect: {
          x: 0,
          y: 0,
          width: img.naturalWidth,
          height: img.naturalHeight,
        },
        maxDimensions,
        preferredFormat,
        compressionGuidance,
        originalFileName: file.name,
      });

      onComplete(result);
    } catch (error) {
      console.error("Failed to process image:", error);
    } finally {
      setIsProcessing(false);
    }
  }, [file, maxDimensions, preferredFormat, compressionGuidance, onComplete]);

  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onCancel()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Crop Image</DialogTitle>
          <DialogDescription>
            Select the area you want to keep. The image will be resized to fit
            within {maxDimensions.width}x{maxDimensions.height} pixels.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center justify-center bg-muted/50 rounded-lg overflow-hidden max-h-[60vh]">
          {imageSrc && (
            <ReactCrop
              crop={crop}
              onChange={(c) => setCrop(c)}
              onComplete={(c) => setCompletedCrop(c)}
              aspect={maxDimensions.width / maxDimensions.height}
            >
              {/* eslint-disable-next-line @next/next/no-img-element -- react-image-crop requires native img element */}
              <img
                ref={imgRef}
                src={imageSrc}
                alt="Crop preview"
                onLoad={onImageLoad}
                className="max-h-[60vh] object-contain"
              />
            </ReactCrop>
          )}
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={onCancel} disabled={isProcessing}>
            Cancel
          </Button>
          <Button variant="secondary" onClick={handleSkip} disabled={isProcessing}>
            Skip Crop
          </Button>
          <Button onClick={handleApply} disabled={isProcessing || !completedCrop}>
            {isProcessing ? "Processing..." : "Apply Crop"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
