"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { Send, Trash2, Wifi, WifiOff, RefreshCw, Upload, Paperclip } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useAgentConsole, useConsoleConfig } from "@/hooks";
import { ConsoleMessage } from "./console-message";
import { AttachmentPreview } from "./attachment-preview";
import { isAllowedType, formatFileSize } from "./attachment-utils";
import type { FileAttachment } from "@/types/websocket";

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

export function AgentConsole({ agentName, namespace, sessionId, className }: Readonly<AgentConsoleProps>) {
  const [input, setInput] = useState("");
  const [attachments, setAttachments] = useState<FileAttachment[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dragCounterRef = useRef(0);

  // Get attachment config from agent's console configuration
  const { config: attachmentConfig } = useConsoleConfig(namespace, agentName);

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
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages]);

  // Process dropped files
  const processFiles = useCallback(async (files: FileList) => {
    const validFiles: FileAttachment[] = [];

    for (const file of Array.from(files)) {
      // Validate type using config
      const typeCheck = isAllowedType(
        file,
        attachmentConfig.allowedMimeTypes,
        attachmentConfig.allowedExtensions
      );
      if (!typeCheck.allowed) continue;

      // Validate size using config
      if (file.size > attachmentConfig.maxFileSize) continue;

      // Convert to data URL
      const dataUrl = await fileToDataUrl(file);

      validFiles.push({
        id: crypto.randomUUID(),
        name: file.name,
        type: file.type,
        size: file.size,
        dataUrl,
      });
    }

    // Limit total files using config
    setAttachments((prev) => [...prev, ...validFiles].slice(0, attachmentConfig.maxFiles));
  }, [attachmentConfig]);

  // Remove attachment
  const removeAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }, []);

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
    if ((input.trim() || attachments.length > 0) && status === "connected") {
      sendMessage(input);
      setInput("");
      setAttachments([]);
      textareaRef.current?.focus();
    }
  }, [input, attachments.length, status, sendMessage]);

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
          {statusBadge}
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

      {/* Messages */}
      <ScrollArea className="flex-1 p-4" ref={scrollRef}>
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
          <div className="flex flex-col gap-4">
            {messages.map((message) => (
              <ConsoleMessage key={message.id} message={message} />
            ))}
          </div>
        )}
      </ScrollArea>

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

        {/* Hidden file input */}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={attachmentConfig.acceptString}
          onChange={handleFileInputChange}
          className="hidden"
          aria-hidden="true"
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
          />
          <Button
            onClick={handleSend}
            disabled={(!input.trim() && attachments.length === 0) || status !== "connected"}
            className="shrink-0"
            aria-label="Send message"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
