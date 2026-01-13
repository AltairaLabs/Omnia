"use client";

import { useCallback, useEffect, useState } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import {
  X,
  ZoomIn,
  ZoomOut,
  Download,
  ChevronLeft,
  ChevronRight,
  RotateCcw,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

interface ImageLightboxProps {
  images: Array<{
    src: string;
    alt: string;
    filename?: string;
  }>;
  initialIndex?: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const MIN_ZOOM = 0.5;
const MAX_ZOOM = 3;
const ZOOM_STEP = 0.25;

function getZoomCursor(zoom: number, isDragging: boolean): string {
  if (zoom <= 1) return "cursor-default";
  if (isDragging) return "cursor-grabbing";
  return "cursor-grab";
}

/**
 * Inner component that handles the lightbox state.
 * Separated to allow key-based remounting for state reset.
 */
function ImageLightboxContent({
  images,
  initialIndex,
}: Readonly<{
  images: ImageLightboxProps["images"];
  initialIndex: number;
}>) {
  const [currentIndex, setCurrentIndex] = useState(initialIndex);
  const [zoom, setZoom] = useState(1);
  const [position, setPosition] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });

  const currentImage = images[currentIndex];
  const hasMultipleImages = images.length > 1;

  const resetZoomAndPosition = useCallback(() => {
    setZoom(1);
    setPosition({ x: 0, y: 0 });
  }, []);

  const handleZoomIn = useCallback(() => {
    setZoom((prev) => Math.min(prev + ZOOM_STEP, MAX_ZOOM));
  }, []);

  const handleZoomOut = useCallback(() => {
    setZoom((prev) => Math.max(prev - ZOOM_STEP, MIN_ZOOM));
  }, []);

  const handlePrevious = useCallback(() => {
    setCurrentIndex((prev) => (prev > 0 ? prev - 1 : images.length - 1));
    resetZoomAndPosition();
  }, [images.length, resetZoomAndPosition]);

  const handleNext = useCallback(() => {
    setCurrentIndex((prev) => (prev < images.length - 1 ? prev + 1 : 0));
    resetZoomAndPosition();
  }, [images.length, resetZoomAndPosition]);

  const handleDownload = useCallback(() => {
    if (!currentImage) return;

    const link = document.createElement("a");
    link.href = currentImage.src;
    link.download = currentImage.filename || `image-${currentIndex + 1}`;
    document.body.appendChild(link);
    link.click();
    link.remove();
  }, [currentImage, currentIndex]);

  // Handle keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      switch (e.key) {
        case "ArrowLeft":
          if (hasMultipleImages) {
            e.preventDefault();
            handlePrevious();
          }
          break;
        case "ArrowRight":
          if (hasMultipleImages) {
            e.preventDefault();
            handleNext();
          }
          break;
        case "+":
        case "=":
          e.preventDefault();
          handleZoomIn();
          break;
        case "-":
          e.preventDefault();
          handleZoomOut();
          break;
        case "0":
          e.preventDefault();
          resetZoomAndPosition();
          break;
      }
    };

    globalThis.addEventListener("keydown", handleKeyDown);
    return () => globalThis.removeEventListener("keydown", handleKeyDown);
  }, [hasMultipleImages, handlePrevious, handleNext, handleZoomIn, handleZoomOut, resetZoomAndPosition]);

  // Handle mouse wheel zoom
  const handleWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    if (e.deltaY < 0) {
      setZoom((prev) => Math.min(prev + ZOOM_STEP, MAX_ZOOM));
    } else {
      setZoom((prev) => Math.max(prev - ZOOM_STEP, MIN_ZOOM));
    }
  }, []);

  // Handle pan when zoomed
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    if (zoom > 1) {
      setIsDragging(true);
      setDragStart({ x: e.clientX - position.x, y: e.clientY - position.y });
    }
  }, [zoom, position]);

  const handleMouseMove = useCallback((e: React.MouseEvent) => {
    if (isDragging && zoom > 1) {
      setPosition({
        x: e.clientX - dragStart.x,
        y: e.clientY - dragStart.y,
      });
    }
  }, [isDragging, zoom, dragStart]);

  const handleMouseUp = useCallback(() => {
    setIsDragging(false);
  }, []);

  if (!currentImage) return null;

  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay
        className="fixed inset-0 z-50 bg-black/90 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <DialogPrimitive.Content
        className="fixed inset-0 z-50 flex flex-col outline-none"
        onPointerDownOutside={(e) => e.preventDefault()}
        aria-describedby={undefined}
        data-testid="image-lightbox"
      >
        {/* Visually hidden title for accessibility */}
        <DialogPrimitive.Title className="sr-only">
          Image viewer
        </DialogPrimitive.Title>

        {/* Header with controls */}
        <div className="flex items-center justify-between p-4 text-white">
          <div className="flex items-center gap-2">
            {hasMultipleImages && (
              <span className="text-sm text-white/70">
                {currentIndex + 1} / {images.length}
              </span>
            )}
          </div>

          <div className="flex items-center gap-1">
            {/* Zoom controls */}
            <Button
              variant="ghost"
              size="icon"
              onClick={handleZoomOut}
              disabled={zoom <= MIN_ZOOM}
              className="text-white hover:bg-white/20 hover:text-white"
              aria-label="Zoom out"
            >
              <ZoomOut className="h-5 w-5" />
            </Button>
            <span className="text-sm text-white/70 min-w-[4rem] text-center">
              {Math.round(zoom * 100)}%
            </span>
            <Button
              variant="ghost"
              size="icon"
              onClick={handleZoomIn}
              disabled={zoom >= MAX_ZOOM}
              className="text-white hover:bg-white/20 hover:text-white"
              aria-label="Zoom in"
            >
              <ZoomIn className="h-5 w-5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={resetZoomAndPosition}
              className="text-white hover:bg-white/20 hover:text-white"
              aria-label="Reset zoom"
            >
              <RotateCcw className="h-5 w-5" />
            </Button>

            <div className="w-px h-6 bg-white/20 mx-2" />

            {/* Download */}
            <Button
              variant="ghost"
              size="icon"
              onClick={handleDownload}
              className="text-white hover:bg-white/20 hover:text-white"
              aria-label="Download image"
            >
              <Download className="h-5 w-5" />
            </Button>

            {/* Close */}
            <DialogPrimitive.Close asChild>
              <Button
                variant="ghost"
                size="icon"
                className="text-white hover:bg-white/20 hover:text-white"
                aria-label="Close"
                data-testid="lightbox-close"
              >
                <X className="h-5 w-5" />
              </Button>
            </DialogPrimitive.Close>
          </div>
        </div>

        {/* Image container */}
        {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions -- Mouse events for pan/drag functionality, keyboard users can use zoom buttons */}
        <div
          className="flex-1 flex items-center justify-center overflow-hidden relative"
          onWheel={handleWheel}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseUp}
        >
          {/* Navigation arrows */}
          {hasMultipleImages && (
            <>
              <Button
                variant="ghost"
                size="icon"
                onClick={handlePrevious}
                className="absolute left-4 z-10 text-white hover:bg-white/20 hover:text-white h-12 w-12"
                aria-label="Previous image"
              >
                <ChevronLeft className="h-8 w-8" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                onClick={handleNext}
                className="absolute right-4 z-10 text-white hover:bg-white/20 hover:text-white h-12 w-12"
                aria-label="Next image"
              >
                <ChevronRight className="h-8 w-8" />
              </Button>
            </>
          )}

          {/* Image */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={currentImage.src}
            alt={currentImage.alt}
            className={cn(
              "max-h-full max-w-full object-contain transition-transform select-none",
              getZoomCursor(zoom, isDragging)
            )}
            style={{
              transform: `scale(${zoom}) translate(${position.x / zoom}px, ${position.y / zoom}px)`,
            }}
            draggable={false}
          />
        </div>

        {/* Footer with filename */}
        {currentImage.filename && (
          <div className="p-4 text-center">
            <span className="text-sm text-white/70">{currentImage.filename}</span>
          </div>
        )}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}

/**
 * Image lightbox component for viewing images in a full-screen modal.
 * Uses key-based remounting to reset state when opened with a new initialIndex.
 */
export function ImageLightbox({
  images,
  initialIndex = 0,
  open,
  onOpenChange,
}: Readonly<ImageLightboxProps>) {
  // Use a key that changes when open+initialIndex changes to reset internal state
  // This is React's recommended pattern for "resetting state when a prop changes"
  // See: https://react.dev/learn/you-might-not-need-an-effect#resetting-all-state-when-a-prop-changes
  const contentKey = open ? `open-${initialIndex}` : "closed";

  if (images.length === 0) return null;

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      {open && (
        <ImageLightboxContent
          key={contentKey}
          images={images}
          initialIndex={initialIndex}
        />
      )}
    </DialogPrimitive.Root>
  );
}
