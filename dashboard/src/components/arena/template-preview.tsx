"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import {
  File,
  FileCode,
  FileText,
  FileJson,
  Copy,
  Check,
} from "lucide-react";
import type { RenderedFile } from "@/types/arena-template";

export interface TemplatePreviewProps {
  files: RenderedFile[];
  className?: string;
}

/**
 * Render icon for file based on extension.
 */
function renderFileIcon(path: string) {
  const className = "h-4 w-4 flex-shrink-0";
  if (path.endsWith(".yaml") || path.endsWith(".yml")) {
    return <FileCode className={className} />;
  }
  if (path.endsWith(".json")) {
    return <FileJson className={className} />;
  }
  if (path.endsWith(".md") || path.endsWith(".txt")) {
    return <FileText className={className} />;
  }
  return <File className={className} />;
}

/**
 * Get language for syntax highlighting based on extension.
 */
function getLanguage(path: string): string {
  if (path.endsWith(".yaml") || path.endsWith(".yml")) {
    return "yaml";
  }
  if (path.endsWith(".json")) {
    return "json";
  }
  if (path.endsWith(".md")) {
    return "markdown";
  }
  return "text";
}

/**
 * File item in the preview tree.
 */
interface FileItemProps {
  file: RenderedFile;
  selected: boolean;
  onSelect: () => void;
}

function FileItem({ file, selected, onSelect }: FileItemProps) {
  const fileName = file.path.split("/").pop() || file.path;

  return (
    <button
      onClick={onSelect}
      className={cn(
        "w-full flex items-center gap-2 px-3 py-2 text-sm text-left rounded-md transition-colors",
        selected
          ? "bg-accent text-accent-foreground"
          : "hover:bg-muted"
      )}
    >
      {renderFileIcon(file.path)}
      <span className="truncate">{fileName}</span>
    </button>
  );
}

/**
 * Code preview with copy functionality.
 */
interface CodePreviewProps {
  content: string;
  language: string;
  path: string;
}

function CodePreview({ content, language, path }: CodePreviewProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const lineCount = content.split("\n").length;

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 border-b bg-muted/50">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">{path}</span>
          <Badge variant="secondary" className="text-xs">
            {language}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {lineCount} line{lineCount === 1 ? "" : "s"}
          </span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={handleCopy}
          className="h-7"
        >
          {copied ? (
            <>
              <Check className="h-3 w-3 mr-1" />
              Copied
            </>
          ) : (
            <>
              <Copy className="h-3 w-3 mr-1" />
              Copy
            </>
          )}
        </Button>
      </div>

      {/* Code */}
      <ScrollArea className="flex-1">
        <pre className="p-4 text-sm font-mono">
          <code>{content}</code>
        </pre>
      </ScrollArea>
    </div>
  );
}

/**
 * Template preview component showing rendered files.
 */
export function TemplatePreview({ files, className }: TemplatePreviewProps) {
  const [selectedIndex, setSelectedIndex] = useState(0);

  if (files.length === 0) {
    return (
      <div className={cn("flex items-center justify-center py-12", className)}>
        <p className="text-muted-foreground">No files to preview</p>
      </div>
    );
  }

  const selectedFile = files[selectedIndex];

  return (
    <div className={cn("flex border rounded-lg overflow-hidden h-[400px]", className)}>
      {/* File list */}
      <div className="w-56 border-r flex flex-col">
        <div className="px-3 py-2 border-b bg-muted/50">
          <span className="text-sm font-medium">Files ({files.length})</span>
        </div>
        <ScrollArea className="flex-1">
          <div className="p-2 space-y-1">
            {files.map((file, index) => (
              <FileItem
                key={file.path}
                file={file}
                selected={index === selectedIndex}
                onSelect={() => setSelectedIndex(index)}
              />
            ))}
          </div>
        </ScrollArea>
      </div>

      {/* Preview pane */}
      <div className="flex-1 flex flex-col">
        {selectedFile ? (
          <CodePreview
            content={selectedFile.content}
            language={getLanguage(selectedFile.path)}
            path={selectedFile.path}
          />
        ) : (
          <div className="flex items-center justify-center flex-1">
            <p className="text-muted-foreground">Select a file to preview</p>
          </div>
        )}
      </div>
    </div>
  );
}
