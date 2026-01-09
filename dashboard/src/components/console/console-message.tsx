"use client";

import { useState } from "react";
import { User, Bot, Loader2, Info, FileDown, FileText, FileCode, FileSpreadsheet } from "lucide-react";
import { cn } from "@/lib/utils";
import { ToolCallCard } from "./tool-call-card";
import { ImageLightbox } from "./image-lightbox";
import type { ConsoleMessage as ConsoleMessageType, FileAttachment } from "@/types/websocket";

interface ConsoleMessageProps {
  message: ConsoleMessageType;
  className?: string;
}

function isImageType(type: string): boolean {
  return type.startsWith("image/");
}

function isAudioType(type: string): boolean {
  return type.startsWith("audio/");
}

function isVideoType(type: string): boolean {
  return type.startsWith("video/");
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getFileIcon(type: string, filename: string) {
  if (type === "application/pdf" || filename.endsWith(".pdf")) {
    return <FileText className="h-4 w-4" />;
  }
  if (type === "application/json" || type === "text/csv" || filename.match(/\.(json|csv)$/)) {
    return <FileSpreadsheet className="h-4 w-4" />;
  }
  if (type.startsWith("text/") || filename.match(/\.(js|ts|jsx|tsx|py|md|txt)$/)) {
    return <FileCode className="h-4 w-4" />;
  }
  return <FileDown className="h-4 w-4" />;
}

function formatTime(date: Date): string {
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function ConsoleMessage({ message, className }: Readonly<ConsoleMessageProps>) {
  const [lightboxOpen, setLightboxOpen] = useState(false);
  const [lightboxIndex, setLightboxIndex] = useState(0);

  const isUser = message.role === "user";
  const isSystem = message.role === "system";

  // Categorize attachments by type
  const imageAttachments = message.attachments?.filter((a) => isImageType(a.type)) ?? [];
  const audioAttachments = message.attachments?.filter((a) => isAudioType(a.type)) ?? [];
  const videoAttachments = message.attachments?.filter((a) => isVideoType(a.type)) ?? [];
  const fileAttachments = message.attachments?.filter(
    (a) => !isImageType(a.type) && !isAudioType(a.type) && !isVideoType(a.type)
  ) ?? [];

  const handleImageClick = (attachment: FileAttachment) => {
    const imageIndex = imageAttachments.findIndex((a) => a.id === attachment.id);
    if (imageIndex !== -1) {
      setLightboxIndex(imageIndex);
      setLightboxOpen(true);
    }
  };

  // System messages render as centered dividers
  if (isSystem) {
    return (
      <div className={cn("flex items-center justify-center gap-2 py-2", className)}>
        <div className="h-px flex-1 bg-border" />
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground px-2">
          <Info className="h-3 w-3" />
          <span>{message.content}</span>
          <span className="text-muted-foreground/60">
            {formatTime(message.timestamp)}
          </span>
        </div>
        <div className="h-px flex-1 bg-border" />
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex gap-3",
        isUser && "flex-row-reverse",
        className
      )}
    >
      {/* Avatar */}
      <div
        className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-full",
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-secondary text-secondary-foreground"
        )}
      >
        {isUser ? (
          <User className="h-4 w-4" />
        ) : (
          <Bot className="h-4 w-4" />
        )}
      </div>

      {/* Content */}
      <div
        className={cn(
          "flex flex-col gap-2 max-w-[80%]",
          isUser && "items-end"
        )}
      >
        <div
          className={cn(
            "rounded-lg px-4 py-2",
            isUser
              ? "bg-primary text-primary-foreground"
              : "bg-secondary text-secondary-foreground"
          )}
        >
          {/* Message content */}
          <div className="whitespace-pre-wrap break-words">
            {message.content}
            {message.isStreaming && message.content.length > 0 && (
              <span className="inline-block w-2 h-4 ml-0.5 bg-current animate-pulse" />
            )}
          </div>

          {/* Streaming indicator for empty content */}
          {message.isStreaming && message.content.length === 0 && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span className="text-sm">Thinking...</span>
            </div>
          )}
        </div>

        {/* Image attachments */}
        {imageAttachments.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {imageAttachments.map((attachment) => (
              <button
                key={attachment.id}
                type="button"
                onClick={() => handleImageClick(attachment)}
                className="relative rounded-lg overflow-hidden cursor-zoom-in focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2"
                aria-label={`View ${attachment.name}`}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={attachment.dataUrl}
                  alt={attachment.name}
                  className="max-w-[200px] max-h-[200px] object-contain"
                />
              </button>
            ))}
          </div>
        )}

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

        {/* Audio attachments */}
        {audioAttachments.length > 0 && (
          <div className="flex flex-col gap-2 w-full">
            {audioAttachments.map((attachment) => (
              <div
                key={attachment.id}
                className="rounded-lg border bg-background/50 p-3"
              >
                <p className="text-xs text-muted-foreground mb-2 truncate" title={attachment.name}>
                  {attachment.name}
                </p>
                <audio
                  controls
                  className="w-full h-10"
                  preload="metadata"
                  aria-label={`Audio: ${attachment.name}`}
                >
                  <source src={attachment.dataUrl} type={attachment.type} />
                  Your browser does not support audio playback.
                </audio>
              </div>
            ))}
          </div>
        )}

        {/* Video attachments */}
        {videoAttachments.length > 0 && (
          <div className="flex flex-col gap-2 w-full">
            {videoAttachments.map((attachment) => (
              <div
                key={attachment.id}
                className="rounded-lg border bg-background/50 overflow-hidden"
              >
                <p className="text-xs text-muted-foreground p-2 truncate" title={attachment.name}>
                  {attachment.name}
                </p>
                <video
                  controls
                  className="w-full max-h-[300px]"
                  preload="metadata"
                  aria-label={`Video: ${attachment.name}`}
                >
                  <source src={attachment.dataUrl} type={attachment.type} />
                  Your browser does not support video playback.
                </video>
              </div>
            ))}
          </div>
        )}

        {/* File attachments (download links) */}
        {fileAttachments.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {fileAttachments.map((attachment) => (
              <a
                key={attachment.id}
                href={attachment.dataUrl}
                download={attachment.name}
                className={cn(
                  "flex items-center gap-2 rounded-lg border px-3 py-2",
                  "bg-background/50 hover:bg-background/80 transition-colors",
                  "text-sm text-foreground no-underline"
                )}
                title={`Download ${attachment.name}`}
              >
                {getFileIcon(attachment.type, attachment.name)}
                <div className="flex flex-col min-w-0">
                  <span className="truncate max-w-[150px]">{attachment.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {formatFileSize(attachment.size)}
                  </span>
                </div>
              </a>
            ))}
          </div>
        )}

        {/* Tool calls */}
        {message.toolCalls && message.toolCalls.length > 0 && (
          <div className="flex flex-col gap-2 w-full">
            {message.toolCalls.map((toolCall) => (
              <ToolCallCard key={toolCall.id} toolCall={toolCall} />
            ))}
          </div>
        )}

        {/* Timestamp */}
        <span className="text-xs text-muted-foreground">
          {formatTime(message.timestamp)}
        </span>
      </div>
    </div>
  );
}
