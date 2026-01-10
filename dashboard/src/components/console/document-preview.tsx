"use client";

import { useState, useMemo } from "react";
import {
  FileText,
  FileSpreadsheet,
  FileCode,
  File,
  FileArchive,
  FileImage,
  Download,
  Eye,
  EyeOff,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

interface DocumentPreviewProps {
  src: string;
  filename: string;
  type: string;
  size: number;
  className?: string;
}

interface FileTypeInfo {
  icon: React.ElementType;
  label: string;
  canPreview: boolean;
}

function getFileTypeInfo(type: string, filename: string): FileTypeInfo {
  // PDF
  if (type === "application/pdf" || filename.endsWith(".pdf")) {
    return { icon: FileText, label: "PDF Document", canPreview: true };
  }

  // Word documents
  if (
    type === "application/msword" ||
    type === "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
    filename.match(/\.(doc|docx)$/i)
  ) {
    return { icon: FileText, label: "Word Document", canPreview: false };
  }

  // Excel/Spreadsheets
  if (
    type === "application/vnd.ms-excel" ||
    type === "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
    type === "text/csv" ||
    filename.match(/\.(xls|xlsx|csv)$/i)
  ) {
    return { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false };
  }

  // PowerPoint
  if (
    type === "application/vnd.ms-powerpoint" ||
    type === "application/vnd.openxmlformats-officedocument.presentationml.presentation" ||
    filename.match(/\.(ppt|pptx)$/i)
  ) {
    return { icon: FileImage, label: "Presentation", canPreview: false };
  }

  // JSON
  if (type === "application/json" || filename.endsWith(".json")) {
    return { icon: FileCode, label: "JSON File", canPreview: true };
  }

  // Code/Text files - check by extension
  const codeExtensions = new Set([
    "js", "ts", "jsx", "tsx", "py", "rb", "go", "rs", "java",
    "c", "cpp", "h", "hpp", "cs", "php", "swift", "kt", "scala",
    "md", "txt", "xml", "yaml", "yml", "toml", "ini", "cfg",
    "conf", "sh", "bash", "zsh", "ps1",
  ]);
  const ext = filename.split(".").pop()?.toLowerCase();
  if (type.startsWith("text/") || (ext && codeExtensions.has(ext))) {
    return { icon: FileCode, label: "Text File", canPreview: true };
  }

  // Archives
  if (
    type === "application/zip" ||
    type === "application/x-zip-compressed" ||
    type === "application/x-rar-compressed" ||
    type === "application/x-7z-compressed" ||
    type === "application/gzip" ||
    type === "application/x-tar" ||
    filename.match(/\.(zip|rar|7z|gz|tar|bz2)$/i)
  ) {
    return { icon: FileArchive, label: "Archive", canPreview: false };
  }

  // Default
  return { icon: File, label: "File", canPreview: false };
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function DocumentPreview({
  src,
  filename,
  type,
  size,
  className,
}: Readonly<DocumentPreviewProps>) {
  const [showPreview, setShowPreview] = useState(false);
  const fileInfo = getFileTypeInfo(type, filename);
  const Icon = fileInfo.icon;
  const isPdf = type === "application/pdf" || filename.endsWith(".pdf");
  const isText = type.startsWith("text/") || type === "application/json";

  return (
    <div
      className={cn(
        "rounded-lg border bg-background/50 overflow-hidden",
        className
      )}
    >
      {/* Header with file info */}
      <div className="flex items-center gap-3 p-3">
        {/* File icon */}
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted">
          <Icon className="h-5 w-5 text-muted-foreground" />
        </div>

        {/* File details */}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate" title={filename}>
            {filename}
          </p>
          <p className="text-xs text-muted-foreground">
            {formatFileSize(size)} • {fileInfo.label}
          </p>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-1">
          {fileInfo.canPreview && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => setShowPreview(!showPreview)}
              aria-label={showPreview ? "Hide preview" : "Show preview"}
              title={showPreview ? "Hide preview" : "Show preview"}
            >
              {showPreview ? (
                <EyeOff className="h-4 w-4" />
              ) : (
                <Eye className="h-4 w-4" />
              )}
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            asChild
          >
            <a
              href={src}
              download={filename}
              aria-label={`Download ${filename}`}
              title={`Download ${filename}`}
            >
              <Download className="h-4 w-4" />
            </a>
          </Button>
        </div>
      </div>

      {/* Preview area */}
      {showPreview && fileInfo.canPreview && (
        <div className="border-t">
          {isPdf ? (
            <PdfPreview src={src} filename={filename} />
          ) : isText ? (
            <TextPreview src={src} />
          ) : null}
        </div>
      )}
    </div>
  );
}

interface PdfPreviewProps {
  src: string;
  filename: string;
}

function PdfPreview({ src, filename }: Readonly<PdfPreviewProps>) {
  return (
    <div className="relative bg-muted/50">
      <object
        data={src}
        type="application/pdf"
        className="w-full h-[300px]"
        aria-label={`Preview of ${filename}`}
      >
        <div className="flex items-center justify-center h-[300px] text-sm text-muted-foreground">
          <p>PDF preview not available in your browser</p>
        </div>
      </object>
    </div>
  );
}

interface TextPreviewProps {
  src: string;
}

function TextPreview({ src }: Readonly<TextPreviewProps>) {
  // Decode base64 content from data URL
  const { content, error } = useMemo(() => {
    try {
      const base64Match = src.match(/^data:[^;]+;base64,(.+)$/);
      if (base64Match) {
        const decoded = atob(base64Match[1]);
        // Limit preview to first 2000 characters
        const truncated = decoded.slice(0, 2000) + (decoded.length > 2000 ? "\n..." : "");
        return { content: truncated, error: false };
      }
      return { content: null, error: true };
    } catch {
      return { content: null, error: true };
    }
  }, [src]);

  if (error) {
    return (
      <div className="flex items-center justify-center h-[150px] text-sm text-muted-foreground">
        <p>Unable to preview file content</p>
      </div>
    );
  }

  return (
    <pre className="p-3 text-xs overflow-auto max-h-[300px] bg-muted/30 font-mono whitespace-pre-wrap break-words">
      {content}
    </pre>
  );
}
