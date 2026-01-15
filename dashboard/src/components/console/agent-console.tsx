"use client";

import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { Send, Trash2, Wifi, WifiOff, RefreshCw, Upload, Paperclip, X, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useAgentConsole, useConsoleConfig } from "@/hooks";
import { ConsoleMessage } from "./console-message";
import { AttachmentPreview } from "./attachment-preview";
import { ImageCropDialog } from "./image-crop-dialog";
import { isAllowedType, formatFileSize, needsResize } from "./attachment-utils";
import { blobToDataUrl, getImageDimensions } from "@/lib/image-processor";
import type { FileAttachment } from "@/types/websocket";

/**
 * Animated thinking indicator shown while waiting for model response.
 */
function ThinkingIndicator() {
  return (
    <div className="flex items-start gap-3 p-3 rounded-lg bg-muted/50">
      <div className="w-8 h-8 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
        <span className="text-xs font-medium text-primary">AI</span>
      </div>
      <div className="flex items-center gap-1 pt-2">
        <span className="w-2 h-2 bg-muted-foreground/60 rounded-full animate-bounce [animation-delay:-0.3s]" />
        <span className="w-2 h-2 bg-muted-foreground/60 rounded-full animate-bounce [animation-delay:-0.15s]" />
        <span className="w-2 h-2 bg-muted-foreground/60 rounded-full animate-bounce" />
      </div>
    </div>
  );
}

interface AgentConsoleProps {
  agentName: string;
  namespace: string;
  /** Optional session ID for multi-tab support */
  sessionId?: string;
  className?: string;
}

function fileToDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

interface FileValidationResult {
  type: "valid" | "rejected" | "needs-crop";
  file: File;
  reason?: string;
}

/**
 * Validates a file against the attachment configuration.
 */
async function validateFile(
  file: File,
  config: { allowedMimeTypes: string[]; allowedExtensions: string[]; maxFileSize: number },
  mediaRequirements?: { image?: { maxDimensions?: { width: number; height: number } } }
): Promise<FileValidationResult> {
  // Validate type
  const typeCheck = isAllowedType(file, config.allowedMimeTypes, config.allowedExtensions);
  if (!typeCheck.allowed) {
    return { type: "rejected", file, reason: typeCheck.reason };
  }

  // Validate size
  if (file.size > config.maxFileSize) {
    return { type: "rejected", file, reason: `File too large (max ${formatFileSize(config.maxFileSize)})` };
  }

  // Check if image needs processing
  if (file.type.startsWith("image/") && mediaRequirements?.image?.maxDimensions) {
    try {
      const dimensions = await getImageDimensions(file);
      const maxDims = mediaRequirements.image.maxDimensions;
      if (needsResize(dimensions.width, dimensions.height, maxDims)) {
        return { type: "needs-crop", file };
      }
    } catch {
      return { type: "rejected", file, reason: "Could not read image dimensions" };
    }
  }

  return { type: "valid", file };
}

export function AgentConsole({ agentName, namespace, sessionId, className }: Readonly<AgentConsoleProps>) {
  const [input, setInput] = useState("");
  const [attachments, setAttachments] = useState<FileAttachment[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const [rejections, setRejections] = useState<string[]>([]);
  const [cropFile, setCropFile] = useState<File | null>(null);
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dragCounterRef = useRef(0);

  // Get attachment config from agent's console configuration
  const { config: attachmentConfig, mediaRequirements } = useConsoleConfig(namespace, agentName);

  // Always use mock mode for now (until K8s integration)
  const {
    sessionId: serverSessionId,
    status,
    messages,
    error,
    sendMessage,
    connect,
    disconnect,
    clearMessages,
  } = useAgentConsole({
    agentName,
    namespace,
    sessionId,
  });

  // Auto-connect on mount
  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  // Process dropped files
  const processFiles = useCallback(async (files: FileList) => {
    const validFiles: FileAttachment[] = [];
    const newRejections: string[] = [];
    const filesToCrop: File[] = [];

    // Validate all files
    const validationResults = await Promise.all(
      Array.from(files).map((file) => validateFile(file, attachmentConfig, mediaRequirements))
    );

    // Process validation results
    for (const result of validationResults) {
      if (result.type === "rejected") {
        newRejections.push(`${result.file.name}: ${result.reason}`);
      } else if (result.type === "needs-crop") {
        filesToCrop.push(result.file);
      } else {
        // Valid file - convert to data URL
        const dataUrl = await fileToDataUrl(result.file);
        validFiles.push({
          id: crypto.randomUUID(),
          name: result.file.name,
          type: result.file.type,
          size: result.file.size,
          dataUrl,
        });
      }
    }

    // Show rejections if any
    if (newRejections.length > 0) {
      setRejections(newRejections);
    }

    // If there are files to crop, show crop dialog for the first one
    if (filesToCrop.length > 0) {
      setCropFile(filesToCrop[0]);
      setPendingFiles(filesToCrop.slice(1));
    }

    // Add valid files to attachments
    if (validFiles.length > 0) {
      setAttachments((prev) => [...prev, ...validFiles].slice(0, attachmentConfig.maxFiles));
    }
  }, [attachmentConfig, mediaRequirements]);

  // Remove attachment
  const removeAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }, []);

  // Clear rejections
  const clearRejections = useCallback(() => {
    setRejections([]);
  }, []);

  // Handle crop completion
  const handleCropComplete = useCallback(async (result: { blob: Blob; file: File }) => {
    const dataUrl = await blobToDataUrl(result.blob);

    const attachment: FileAttachment = {
      id: crypto.randomUUID(),
      name: result.file.name,
      type: result.file.type,
      size: result.blob.size,
      dataUrl,
    };

    setAttachments((prev) => [...prev, attachment].slice(0, attachmentConfig.maxFiles));
    setCropFile(null);

    // Process next pending file
    if (pendingFiles.length > 0) {
      setCropFile(pendingFiles[0]);
      setPendingFiles((prev) => prev.slice(1));
    }
  }, [attachmentConfig.maxFiles, pendingFiles]);

  // Handle crop cancel
  const handleCropCancel = useCallback(() => {
    setCropFile(null);
    // Process next pending file
    if (pendingFiles.length > 0) {
      setCropFile(pendingFiles[0]);
      setPendingFiles((prev) => prev.slice(1));
    }
  }, [pendingFiles]);

  // Drag event handlers
  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current++;
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragging(true);
    }
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current--;
    if (dragCounterRef.current === 0) {
      setIsDragging(false);
    }
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      dragCounterRef.current = 0;
      setIsDragging(false);

      if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
        processFiles(e.dataTransfer.files);
      }
    },
    [processFiles]
  );

  // Handle send - only sends if canSend is true
  const handleSend = useCallback(() => {
    const hasContent = input.trim().length > 0 || attachments.length > 0;
    if (!hasContent || status !== "connected") {
      return;
    }
    sendMessage(input, attachments);
    setInput("");
    setAttachments([]);
    textareaRef.current?.focus();
  }, [input, attachments, status, sendMessage]);

  // Handle key press
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend]
  );

  // Handle paste - extract images from clipboard
  const handlePaste = useCallback(
    async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      const items = e.clipboardData?.items;
      if (!items) return;

      const imageFiles: File[] = [];

      for (const item of Array.from(items)) {
        // Check if item is an image with allowed type
        if (item.type.startsWith("image/")) {
          const ext = item.type.split("/")[1] || "png";
          const tempName = `pasted.${ext}`;
          const typeCheck = isAllowedType(
            { type: item.type, name: tempName },
            attachmentConfig.allowedMimeTypes,
            attachmentConfig.allowedExtensions
          );
          if (typeCheck.allowed) {
            const file = item.getAsFile();
            if (file && file.size <= attachmentConfig.maxFileSize) {
              imageFiles.push(file);
            }
          }
        }
      }

      // If we found images, process them
      if (imageFiles.length > 0) {
        // Prevent default only if we're handling images
        // This allows normal text paste to work
        e.preventDefault();

        const newAttachments: FileAttachment[] = [];

        for (const file of imageFiles) {
          const dataUrl = await fileToDataUrl(file);
          newAttachments.push({
            id: crypto.randomUUID(),
            // Generate a name for pasted images (they don't have one)
            name: `pasted-image-${Date.now()}.${file.type.split("/")[1] || "png"}`,
            type: file.type,
            size: file.size,
            dataUrl,
          });
        }

        // Add to attachments, respecting max limit
        setAttachments((prev) => [...prev, ...newAttachments].slice(0, attachmentConfig.maxFiles));
      }
    },
    [attachmentConfig]
  );

  // Handle file input change (from attachment button)
  const handleFileInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files && e.target.files.length > 0) {
        processFiles(e.target.files);
      }
      // Reset input so the same file can be selected again
      e.target.value = "";
    },
    [processFiles]
  );

  // Handle attachment button click
  const handleAttachmentClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  // Determine if we should show the thinking indicator
  // Show when the last message is from the user (waiting for assistant response)
  // Hide if there's an error or we're disconnected
  const isWaitingForResponse = useMemo(() => {
    if (messages.length === 0) return false;
    if (status === "error" || status === "disconnected") return false;
    const lastMessage = messages[messages.length - 1];
    return lastMessage.role === "user";
  }, [messages, status]);

  // Determine if we can send a message (text or attachments present, and connected)
  const canSend = useMemo(() => {
    const hasContent = input.trim().length > 0 || attachments.length > 0;
    return hasContent && status === "connected";
  }, [input, attachments, status]);

  // Status badge
  const statusBadge = {
    disconnected: (
      <Badge variant="outline" className="gap-1.5">
        <WifiOff className="h-3 w-3" />
        Disconnected
      </Badge>
    ),
    connecting: (
      <Badge variant="outline" className="gap-1.5 text-yellow-600 border-yellow-600/30 bg-yellow-500/10">
        <RefreshCw className="h-3 w-3 animate-spin" />
        Connecting
      </Badge>
    ),
    connected: (
      <Badge variant="outline" className="gap-1.5 text-green-600 border-green-600/30 bg-green-500/10">
        <Wifi className="h-3 w-3" />
        Connected
      </Badge>
    ),
    error: (
      <Badge variant="outline" className="gap-1.5 text-red-600 border-red-600/30 bg-red-500/10">
        <WifiOff className="h-3 w-3" />
        Error
      </Badge>
    ),
  }[status];

  return (
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions -- Drag events for file drop zone, keyboard users can use the attach file button instead
    <div
      className={cn("flex flex-col h-[600px] border rounded-lg relative", className)}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      data-testid="console-dropzone"
    >
      {/* Drop zone overlay */}
      {isDragging && (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm border-2 border-dashed border-primary rounded-lg">
          <div className="flex flex-col items-center gap-2 text-primary">
            <Upload className="h-12 w-12" />
            <p className="text-lg font-medium">Drop files here</p>
            <p className="text-sm text-muted-foreground">
              Files up to {formatFileSize(attachmentConfig.maxFileSize)}
            </p>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-muted/30">
        <div className="flex items-center gap-3">
          <span data-testid="connection-status">{statusBadge}</span>
          {serverSessionId && (
            <span className="text-xs text-muted-foreground">
              Session: {serverSessionId.slice(0, 12)}...
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {status === "disconnected" && (
            <Button variant="outline" size="sm" onClick={connect}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Reconnect
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            onClick={clearMessages}
            disabled={messages.length === 0}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="px-4 py-2 bg-red-500/10 border-b border-red-500/20 text-red-600 dark:text-red-400 text-sm">
          {error}
        </div>
      )}

      {/* File rejection feedback */}
      {rejections.length > 0 && (
        <div className="px-4 py-2 bg-amber-500/10 border-b border-amber-500/20" data-testid="rejection-feedback">
          <div className="flex items-start gap-2">
            <AlertCircle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
            <div className="flex-1 text-sm text-amber-600 dark:text-amber-400">
              <p className="font-medium">Some files could not be added:</p>
              <ul className="mt-1 list-disc list-inside">
                {rejections.map((reason, index) => (
                  <li key={index}>{reason}</li>
                ))}
              </ul>
            </div>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 shrink-0"
              onClick={clearRejections}
              aria-label="Dismiss"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4">
        {messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
            <p className="text-center">
              Start a conversation with <strong>{agentName}</strong>
            </p>
            <p className="text-sm mt-1">
              Type a message below and press Enter to send
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-4" data-testid="message-list">
            {messages.map((message) => (
              <ConsoleMessage key={message.id} message={message} />
            ))}
            {/* Thinking indicator while waiting for response */}
            {isWaitingForResponse && <ThinkingIndicator />}
            {/* Sentinel element for auto-scroll */}
            <div ref={messagesEndRef} />
          </div>
        )}
      </div>

      {/* Input area */}
      <div className="p-4 border-t bg-muted/30">
        {/* Attachment preview */}
        {attachments.length > 0 && (
          <AttachmentPreview
            attachments={attachments}
            onRemove={removeAttachment}
            className="mb-3"
          />
        )}

        {/* Hidden file input - visually hidden but accessible for programmatic clicks */}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={attachmentConfig.acceptString}
          onChange={handleFileInputChange}
          className="hidden"
          tabIndex={-1}
        />

        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={handleAttachmentClick}
            disabled={status !== "connected" || attachments.length >= attachmentConfig.maxFiles}
            className="shrink-0"
            aria-label="Attach files"
            data-testid="attachment-button"
          >
            <Paperclip className="h-4 w-4" />
          </Button>
          <Textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            placeholder={
              status === "connected"
                ? "Type a message... (Enter to send, Shift+Enter for new line)"
                : "Connect to start chatting..."
            }
            disabled={status !== "connected"}
            className="min-h-[44px] max-h-[120px] resize-none"
            rows={1}
            data-testid="console-input"
          />
          <Button
            onClick={handleSend}
            disabled={!canSend}
            className="shrink-0"
            aria-label="Send message"
            data-testid="send-button"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Image crop dialog */}
      {cropFile && mediaRequirements?.image?.maxDimensions && (
        <ImageCropDialog
          file={cropFile}
          maxDimensions={mediaRequirements.image.maxDimensions}
          preferredFormat={mediaRequirements.image.preferredFormat as "image/jpeg" | "image/png" | "image/webp" | undefined}
          compressionGuidance={mediaRequirements.image.compressionGuidance}
          onComplete={handleCropComplete}
          onCancel={handleCropCancel}
          open={!!cropFile}
        />
      )}
    </div>
  );
}
