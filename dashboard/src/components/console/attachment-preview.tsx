"use client";

import { useState } from "react";
import { X, FileAudio, FileImage, File } from "lucide-react";
import { cn } from "@/lib/utils";
import { ImageLightbox } from "./image-lightbox";
import type { FileAttachment } from "@/types/websocket";

interface AttachmentPreviewProps {
  attachments: FileAttachment[];
  onRemove?: (id: string) => void;
  className?: string;
  readonly?: boolean;
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getFileIcon(type: string) {
  if (type.startsWith("image/")) return FileImage;
  if (type.startsWith("audio/")) return FileAudio;
  return File;
}

function isImageType(type: string): boolean {
  return type.startsWith("image/");
}

export function AttachmentPreview({
  attachments,
  onRemove,
  className,
  readonly = false,
}: Readonly<AttachmentPreviewProps>) {
  const [lightboxOpen, setLightboxOpen] = useState(false);
  const [lightboxIndex, setLightboxIndex] = useState(0);

  // Get all image attachments for the lightbox
  const imageAttachments = attachments.filter((a) => isImageType(a.type));

  const handleImageClick = (attachment: FileAttachment) => {
    const imageIndex = imageAttachments.findIndex((a) => a.id === attachment.id);
    if (imageIndex !== -1) {
      setLightboxIndex(imageIndex);
      setLightboxOpen(true);
    }
  };

  if (attachments.length === 0) return null;

  return (
    <div
      className={cn(
        "flex gap-2 overflow-x-auto pb-2",
        className
      )}
    >
      {attachments.map((attachment) => {
        const FileIcon = getFileIcon(attachment.type);
        const isImage = isImageType(attachment.type);

        return (
          <div
            key={attachment.id}
            className={cn(
              "relative group flex-shrink-0 rounded-lg border bg-muted/50 overflow-hidden",
              isImage ? "w-20 h-20" : "flex items-center gap-2 px-3 py-2"
            )}
          >
            {isImage ? (
              <button
                type="button"
                onClick={() => handleImageClick(attachment)}
                className="w-full h-full cursor-zoom-in focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2"
                aria-label={`View ${attachment.name}`}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={attachment.dataUrl}
                  alt={attachment.name}
                  className="w-full h-full object-cover"
                />
              </button>
            ) : (
              <>
                <FileIcon className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                <div className="flex flex-col min-w-0">
                  <span className="text-xs font-medium truncate max-w-[120px]">
                    {attachment.name}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {formatFileSize(attachment.size)}
                  </span>
                </div>
              </>
            )}

            {/* Remove button - pointer-events-none when invisible to not block clicks */}
            {!readonly && onRemove && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onRemove(attachment.id);
                }}
                className={cn(
                  "absolute top-1 right-1 p-0.5 rounded-full z-10",
                  "bg-background/80 hover:bg-destructive hover:text-destructive-foreground",
                  "opacity-0 group-hover:opacity-100 transition-opacity",
                  "pointer-events-none group-hover:pointer-events-auto",
                  "focus:opacity-100 focus:pointer-events-auto focus:outline-none focus:ring-2 focus:ring-ring"
                )}
                aria-label={`Remove ${attachment.name}`}
              >
                <X className="h-3 w-3" />
              </button>
            )}

            {/* Image overlay with file info on hover - pointer-events-none to not block clicks */}
            {isImage && (
              <div
                className={cn(
                  "absolute inset-x-0 bottom-0 px-1 py-0.5 pointer-events-none",
                  "bg-background/80 text-xs truncate",
                  "opacity-0 group-hover:opacity-100 transition-opacity"
                )}
              >
                {attachment.name}
              </div>
            )}
          </div>
        );
      })}

      {/* Image Lightbox */}
      {imageAttachments.length > 0 && (
        <ImageLightbox
          images={imageAttachments.map((a) => ({
            src: a.dataUrl,
            alt: a.name,
            filename: a.name,
          }))}
          initialIndex={lightboxIndex}
          open={lightboxOpen}
          onOpenChange={setLightboxOpen}
        />
      )}
    </div>
  );
}
