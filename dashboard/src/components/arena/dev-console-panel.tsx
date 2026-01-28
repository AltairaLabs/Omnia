"use client";

import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import {
  Send,
  Trash2,
  Wifi,
  WifiOff,
  RefreshCw,
  Upload,
  Paperclip,
  X,
  AlertCircle,
  RotateCcw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useDevConsole } from "@/hooks";
import { ConsoleMessage } from "@/components/console/console-message";
import { AttachmentPreview } from "@/components/console/attachment-preview";
import { ImageCropDialog } from "@/components/console/image-crop-dialog";
import {
  isAllowedType,
  formatFileSize,
  needsResize,
} from "@/components/console/attachment-utils";
import { blobToDataUrl, getImageDimensions } from "@/lib/image-processor";
import type { FileAttachment } from "@/types/websocket";

// =============================================================================
// Types
// =============================================================================

interface DevConsolePanelProps {
  /** Project ID for the dev console session */
  projectId: string;
  /** Workspace name */
  workspace?: string;
  /** Path to the agent configuration file (for reload) */
  configPath?: string;
  /** Available providers for selection */
  providers?: Array<{ id: string; name: string }>;
  /** Selected provider ID */
  selectedProvider?: string;
  /** Callback when provider selection changes */
  onProviderChange?: (providerId: string) => void;
  /** Optional class name */
  className?: string;
}

interface FileValidationResult {
  type: "valid" | "rejected" | "needs-crop";
  file: File;
  reason?: string;
}

// =============================================================================
// Constants
// =============================================================================

// Default attachment configuration for dev console
const DEFAULT_ATTACHMENT_CONFIG = {
  allowedMimeTypes: [
    "image/jpeg",
    "image/png",
    "image/gif",
    "image/webp",
    "application/pdf",
    "text/plain",
    "application/json",
  ],
  allowedExtensions: [
    ".jpg",
    ".jpeg",
    ".png",
    ".gif",
    ".webp",
    ".pdf",
    ".txt",
    ".json",
  ],
  maxFileSize: 10 * 1024 * 1024, // 10MB
  maxFiles: 5,
  acceptString:
    "image/jpeg,image/png,image/gif,image/webp,application/pdf,text/plain,application/json",
};

const DEFAULT_MEDIA_REQUIREMENTS = {
  image: {
    maxDimensions: { width: 2048, height: 2048 },
    preferredFormat: "image/webp" as const,
  },
};

// =============================================================================
// Helpers
// =============================================================================

function fileToDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

async function validateFile(
  file: File,
  config: typeof DEFAULT_ATTACHMENT_CONFIG,
  mediaRequirements?: typeof DEFAULT_MEDIA_REQUIREMENTS
): Promise<FileValidationResult> {
  // Validate type
  const typeCheck = isAllowedType(
    file,
    config.allowedMimeTypes,
    config.allowedExtensions
  );
  if (!typeCheck.allowed) {
    return { type: "rejected", file, reason: typeCheck.reason };
  }

  // Validate size
  if (file.size > config.maxFileSize) {
    return {
      type: "rejected",
      file,
      reason: `File too large (max ${formatFileSize(config.maxFileSize)})`,
    };
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

// =============================================================================
// Components
// =============================================================================

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

/**
 * Dev Console Panel - Interactive agent testing in the project editor.
 * Reuses the console UI components with dev console-specific features.
 */
export function DevConsolePanel({
  projectId,
  workspace,
  configPath,
  providers = [],
  selectedProvider,
  onProviderChange,
  className,
}: Readonly<DevConsolePanelProps>) {
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

  // Use the dev console hook
  const {
    sessionId,
    status,
    messages,
    error,
    sendMessage,
    connect,
    disconnect,
    clearMessages,
    reload,
    resetConversation,
    setProvider,
  } = useDevConsole({
    sessionId: `project-${projectId}`,
    projectId,
    workspace,
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

  // Handle provider change
  const handleProviderChange = useCallback(
    (providerId: string) => {
      setProvider(providerId);
      onProviderChange?.(providerId);
    },
    [setProvider, onProviderChange]
  );

  // Handle reload
  const handleReload = useCallback(() => {
    if (configPath) {
      reload(configPath);
    }
  }, [reload, configPath]);

  // Handle reset conversation
  const handleReset = useCallback(() => {
    resetConversation();
  }, [resetConversation]);

  // Process dropped files
  const processFiles = useCallback(async (files: FileList) => {
    const validFiles: FileAttachment[] = [];
    const newRejections: string[] = [];
    const filesToCrop: File[] = [];

    const validationResults = await Promise.all(
      Array.from(files).map((file) =>
        validateFile(file, DEFAULT_ATTACHMENT_CONFIG, DEFAULT_MEDIA_REQUIREMENTS)
      )
    );

    for (const result of validationResults) {
      if (result.type === "rejected") {
        newRejections.push(`${result.file.name}: ${result.reason}`);
      } else if (result.type === "needs-crop") {
        filesToCrop.push(result.file);
      } else {
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

    if (newRejections.length > 0) {
      setRejections(newRejections);
    }

    if (filesToCrop.length > 0) {
      setCropFile(filesToCrop[0]);
      setPendingFiles(filesToCrop.slice(1));
    }

    if (validFiles.length > 0) {
      setAttachments((prev) =>
        [...prev, ...validFiles].slice(0, DEFAULT_ATTACHMENT_CONFIG.maxFiles)
      );
    }
  }, []);

  // Remove attachment
  const removeAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }, []);

  // Clear rejections
  const clearRejections = useCallback(() => {
    setRejections([]);
  }, []);

  // Handle crop completion
  const handleCropComplete = useCallback(
    async (result: { blob: Blob; file: File }) => {
      const dataUrl = await blobToDataUrl(result.blob);

      const attachment: FileAttachment = {
        id: crypto.randomUUID(),
        name: result.file.name,
        type: result.file.type,
        size: result.blob.size,
        dataUrl,
      };

      setAttachments((prev) =>
        [...prev, attachment].slice(0, DEFAULT_ATTACHMENT_CONFIG.maxFiles)
      );
      setCropFile(null);

      if (pendingFiles.length > 0) {
        setCropFile(pendingFiles[0]);
        setPendingFiles((prev) => prev.slice(1));
      }
    },
    [pendingFiles]
  );

  // Handle crop cancel
  const handleCropCancel = useCallback(() => {
    setCropFile(null);
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

  // Handle send
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

  // Handle paste
  const handlePaste = useCallback(
    async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      const items = e.clipboardData?.items;
      if (!items) return;

      const imageFiles: File[] = [];

      for (const item of Array.from(items)) {
        if (item.type.startsWith("image/")) {
          const ext = item.type.split("/")[1] || "png";
          const tempName = `pasted.${ext}`;
          const typeCheck = isAllowedType(
            { type: item.type, name: tempName },
            DEFAULT_ATTACHMENT_CONFIG.allowedMimeTypes,
            DEFAULT_ATTACHMENT_CONFIG.allowedExtensions
          );
          if (typeCheck.allowed) {
            const file = item.getAsFile();
            if (file && file.size <= DEFAULT_ATTACHMENT_CONFIG.maxFileSize) {
              imageFiles.push(file);
            }
          }
        }
      }

      if (imageFiles.length > 0) {
        e.preventDefault();

        const newAttachments: FileAttachment[] = [];

        for (const file of imageFiles) {
          const dataUrl = await fileToDataUrl(file);
          newAttachments.push({
            id: crypto.randomUUID(),
            name: `pasted-image-${Date.now()}.${file.type.split("/")[1] || "png"}`,
            type: file.type,
            size: file.size,
            dataUrl,
          });
        }

        setAttachments((prev) =>
          [...prev, ...newAttachments].slice(0, DEFAULT_ATTACHMENT_CONFIG.maxFiles)
        );
      }
    },
    []
  );

  // Handle file input change
  const handleFileInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files && e.target.files.length > 0) {
        processFiles(e.target.files);
      }
      e.target.value = "";
    },
    [processFiles]
  );

  // Handle attachment button click
  const handleAttachmentClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  // Determine if we should show the thinking indicator
  const isWaitingForResponse = useMemo(() => {
    if (messages.length === 0) return false;
    if (status === "error" || status === "disconnected") return false;
    const lastMessage = messages[messages.length - 1];
    return lastMessage.role === "user";
  }, [messages, status]);

  // Determine if we can send a message
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
      <Badge
        variant="outline"
        className="gap-1.5 text-yellow-600 border-yellow-600/30 bg-yellow-500/10"
      >
        <RefreshCw className="h-3 w-3 animate-spin" />
        Connecting
      </Badge>
    ),
    connected: (
      <Badge
        variant="outline"
        className="gap-1.5 text-green-600 border-green-600/30 bg-green-500/10"
      >
        <Wifi className="h-3 w-3" />
        Connected
      </Badge>
    ),
    error: (
      <Badge
        variant="outline"
        className="gap-1.5 text-red-600 border-red-600/30 bg-red-500/10"
      >
        <WifiOff className="h-3 w-3" />
        Error
      </Badge>
    ),
  }[status];

  return (
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      className={cn("flex flex-col h-full relative", className)}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      data-testid="dev-console-dropzone"
    >
      {/* Drop zone overlay */}
      {isDragging && (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm border-2 border-dashed border-primary rounded-lg">
          <div className="flex flex-col items-center gap-2 text-primary">
            <Upload className="h-12 w-12" />
            <p className="text-lg font-medium">Drop files here</p>
            <p className="text-sm text-muted-foreground">
              Files up to {formatFileSize(DEFAULT_ATTACHMENT_CONFIG.maxFileSize)}
            </p>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b bg-muted/30">
        <div className="flex items-center gap-3">
          <span data-testid="connection-status">{statusBadge}</span>
          {sessionId && (
            <span className="text-xs text-muted-foreground">
              Session: {sessionId.slice(0, 12)}...
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {/* Provider selector */}
          {providers.length > 0 && (
            <Select
              value={selectedProvider}
              onValueChange={handleProviderChange}
              disabled={status !== "connected"}
            >
              <SelectTrigger className="h-7 w-[140px] text-xs">
                <SelectValue placeholder="Select provider" />
              </SelectTrigger>
              <SelectContent>
                {providers.map((provider) => (
                  <SelectItem key={provider.id} value={provider.id}>
                    {provider.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          {/* Reload button */}
          {configPath && (
            <Button
              variant="ghost"
              size="sm"
              onClick={handleReload}
              disabled={status !== "connected"}
              title="Reload configuration"
              className="h-7"
            >
              <RefreshCw className="h-3.5 w-3.5" />
            </Button>
          )}

          {/* Reset conversation button */}
          <Button
            variant="ghost"
            size="sm"
            onClick={handleReset}
            disabled={status !== "connected" || messages.length === 0}
            title="Reset conversation"
            className="h-7"
          >
            <RotateCcw className="h-3.5 w-3.5" />
          </Button>

          {/* Clear messages button */}
          <Button
            variant="ghost"
            size="sm"
            onClick={clearMessages}
            disabled={messages.length === 0}
            title="Clear messages"
            className="h-7"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>

          {/* Reconnect button */}
          {status === "disconnected" && (
            <Button variant="outline" size="sm" onClick={connect} className="h-7">
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              Reconnect
            </Button>
          )}
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="px-3 py-1.5 bg-red-500/10 border-b border-red-500/20 text-red-600 dark:text-red-400 text-xs">
          {error}
        </div>
      )}

      {/* File rejection feedback */}
      {rejections.length > 0 && (
        <div
          className="px-3 py-1.5 bg-amber-500/10 border-b border-amber-500/20"
          data-testid="rejection-feedback"
        >
          <div className="flex items-start gap-2">
            <AlertCircle className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
            <div className="flex-1 text-xs text-amber-600 dark:text-amber-400">
              <p className="font-medium">Some files could not be added:</p>
              <ul className="mt-0.5 list-disc list-inside">
                {rejections.map((reason) => (
                  <li key={reason}>{reason}</li>
                ))}
              </ul>
            </div>
            <Button
              variant="ghost"
              size="icon"
              className="h-5 w-5 shrink-0"
              onClick={clearRejections}
              aria-label="Dismiss"
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      )}

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-3 min-h-0">
        {messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
            <p className="text-center text-sm">Test your agent</p>
            <p className="text-xs mt-1">
              Type a message below and press Enter to send
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-3" data-testid="message-list">
            {messages.map((message) => (
              <ConsoleMessage key={message.id} message={message} />
            ))}
            {isWaitingForResponse && <ThinkingIndicator />}
            <div ref={messagesEndRef} />
          </div>
        )}
      </div>

      {/* Input area */}
      <div className="p-3 border-t bg-muted/30">
        {/* Attachment preview */}
        {attachments.length > 0 && (
          <AttachmentPreview
            attachments={attachments}
            onRemove={removeAttachment}
            className="mb-2"
          />
        )}

        {/* Hidden file input */}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={DEFAULT_ATTACHMENT_CONFIG.acceptString}
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
            disabled={
              status !== "connected" ||
              attachments.length >= DEFAULT_ATTACHMENT_CONFIG.maxFiles
            }
            className="shrink-0 h-8 w-8"
            aria-label="Attach files"
            data-testid="attachment-button"
          >
            <Paperclip className="h-3.5 w-3.5" />
          </Button>
          <Textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            placeholder={
              status === "connected"
                ? "Type a message... (Enter to send)"
                : "Connect to start testing..."
            }
            disabled={status !== "connected"}
            className="min-h-[36px] max-h-[80px] resize-none text-sm"
            rows={1}
            data-testid="console-input"
          />
          <Button
            onClick={handleSend}
            disabled={!canSend}
            className="shrink-0 h-8 w-8"
            size="icon"
            aria-label="Send message"
            data-testid="send-button"
          >
            <Send className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Image crop dialog */}
      {cropFile && DEFAULT_MEDIA_REQUIREMENTS.image?.maxDimensions && (
        <ImageCropDialog
          file={cropFile}
          maxDimensions={DEFAULT_MEDIA_REQUIREMENTS.image.maxDimensions}
          preferredFormat={DEFAULT_MEDIA_REQUIREMENTS.image.preferredFormat}
          onComplete={handleCropComplete}
          onCancel={handleCropCancel}
          open={!!cropFile}
        />
      )}
    </div>
  );
}
