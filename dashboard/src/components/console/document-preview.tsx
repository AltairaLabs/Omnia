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

// File size constants
const BYTES_PER_KB = 1024;
const BYTES_PER_MB = 1024 * 1024;

// Preview constants
const TEXT_PREVIEW_MAX_CHARS = 2000;
const PDF_PREVIEW_HEIGHT = 300;
const TEXT_PREVIEW_HEIGHT = 300;
const ERROR_DISPLAY_HEIGHT = 150;

// Code/text file extensions that support preview
const CODE_EXTENSIONS = new Set([
  "js", "ts", "jsx", "tsx", "py", "rb", "go", "rs", "java",
  "c", "cpp", "h", "hpp", "cs", "php", "swift", "kt", "scala",
  "md", "txt", "xml", "yaml", "yml", "toml", "ini", "cfg",
  "conf", "sh", "bash", "zsh", "ps1",
]);

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

// MIME type to file info mapping
const MIME_TYPE_MAP: Record<string, FileTypeInfo> = {
  "application/pdf": { icon: FileText, label: "PDF Document", canPreview: true },
  "application/msword": { icon: FileText, label: "Word Document", canPreview: false },
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document": { icon: FileText, label: "Word Document", canPreview: false },
  "application/vnd.ms-excel": { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false },
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false },
  "text/csv": { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false },
  "application/vnd.ms-powerpoint": { icon: FileImage, label: "Presentation", canPreview: false },
  "application/vnd.openxmlformats-officedocument.presentationml.presentation": { icon: FileImage, label: "Presentation", canPreview: false },
  "application/json": { icon: FileCode, label: "JSON File", canPreview: true },
  "application/zip": { icon: FileArchive, label: "Archive", canPreview: false },
  "application/x-zip-compressed": { icon: FileArchive, label: "Archive", canPreview: false },
  "application/x-rar-compressed": { icon: FileArchive, label: "Archive", canPreview: false },
  "application/x-7z-compressed": { icon: FileArchive, label: "Archive", canPreview: false },
  "application/gzip": { icon: FileArchive, label: "Archive", canPreview: false },
  "application/x-tar": { icon: FileArchive, label: "Archive", canPreview: false },
};

// Extension to file info mapping for fallback detection
const EXTENSION_MAP: Record<string, FileTypeInfo> = {
  pdf: { icon: FileText, label: "PDF Document", canPreview: true },
  doc: { icon: FileText, label: "Word Document", canPreview: false },
  docx: { icon: FileText, label: "Word Document", canPreview: false },
  xls: { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false },
  xlsx: { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false },
  csv: { icon: FileSpreadsheet, label: "Spreadsheet", canPreview: false },
  ppt: { icon: FileImage, label: "Presentation", canPreview: false },
  pptx: { icon: FileImage, label: "Presentation", canPreview: false },
  json: { icon: FileCode, label: "JSON File", canPreview: true },
  zip: { icon: FileArchive, label: "Archive", canPreview: false },
  rar: { icon: FileArchive, label: "Archive", canPreview: false },
  "7z": { icon: FileArchive, label: "Archive", canPreview: false },
  gz: { icon: FileArchive, label: "Archive", canPreview: false },
  tar: { icon: FileArchive, label: "Archive", canPreview: false },
  bz2: { icon: FileArchive, label: "Archive", canPreview: false },
};

const DEFAULT_FILE_INFO: FileTypeInfo = { icon: File, label: "File", canPreview: false };
const TEXT_FILE_INFO: FileTypeInfo = { icon: FileCode, label: "Text File", canPreview: true };

function getFileTypeInfo(type: string, filename: string): FileTypeInfo {
  // Check MIME type first
  if (MIME_TYPE_MAP[type]) {
    return MIME_TYPE_MAP[type];
  }

  // Check text types
  if (type.startsWith("text/")) {
    return TEXT_FILE_INFO;
  }

  // Fall back to extension-based detection
  const ext = filename.split(".").pop()?.toLowerCase();
  if (ext) {
    if (EXTENSION_MAP[ext]) {
      return EXTENSION_MAP[ext];
    }
    if (CODE_EXTENSIONS.has(ext)) {
      return TEXT_FILE_INFO;
    }
  }

  return DEFAULT_FILE_INFO;
}

function formatFileSize(bytes: number): string {
  if (bytes < BYTES_PER_KB) return `${bytes} B`;
  if (bytes < BYTES_PER_MB) return `${(bytes / BYTES_PER_KB).toFixed(1)} KB`;
  return `${(bytes / BYTES_PER_MB).toFixed(1)} MB`;
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
      data-testid="document-preview"
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
          {isPdf && <PdfPreview src={src} filename={filename} />}
          {isText && !isPdf && <TextPreview src={src} />}
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
        className="w-full"
        style={{ height: PDF_PREVIEW_HEIGHT }}
        aria-label={`Preview of ${filename}`}
      >
        <div
          className="flex items-center justify-center text-sm text-muted-foreground"
          style={{ height: PDF_PREVIEW_HEIGHT }}
        >
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
      const base64Regex = /^data:[^;]+;base64,(.+)$/;
      const base64Match = base64Regex.exec(src);
      if (base64Match) {
        const decoded = atob(base64Match[1]);
        const truncated = decoded.length > TEXT_PREVIEW_MAX_CHARS
          ? decoded.slice(0, TEXT_PREVIEW_MAX_CHARS) + "\n..."
          : decoded;
        return { content: truncated, error: false };
      }
      return { content: null, error: true };
    } catch {
      return { content: null, error: true };
    }
  }, [src]);

  if (error) {
    return (
      <div
        className="flex items-center justify-center text-sm text-muted-foreground"
        style={{ height: ERROR_DISPLAY_HEIGHT }}
      >
        <p>Unable to preview file content</p>
      </div>
    );
  }

  return (
    <pre
      className="p-3 text-xs overflow-auto bg-muted/30 font-mono whitespace-pre-wrap break-words"
      style={{ maxHeight: TEXT_PREVIEW_HEIGHT }}
    >
      {content}
    </pre>
  );
}
