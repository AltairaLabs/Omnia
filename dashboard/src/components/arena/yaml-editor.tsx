"use client";

import { useCallback, useRef, useMemo } from "react";
import Editor, { type OnMount, type OnChange } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import * as yaml from "js-yaml";
import { cn } from "@/lib/utils";
import { Loader2, AlertTriangle, CheckCircle } from "lucide-react";
import type { FileType } from "@/types/arena-project";

interface YamlEditorProps {
  readonly value: string;
  readonly onChange?: (value: string) => void;
  readonly onSave?: () => void;
  readonly readOnly?: boolean;
  readonly language?: string;
  readonly fileType?: FileType;
  readonly className?: string;
  readonly loading?: boolean;
}

interface ValidationResult {
  valid: boolean;
  error?: string;
  line?: number;
}

/**
 * Get Monaco language from file type
 */
function getLanguage(fileType: FileType | undefined): string {
  switch (fileType) {
    case "arena":
    case "prompt":
    case "provider":
    case "scenario":
    case "tool":
    case "persona":
    case "yaml":
      return "yaml";
    case "json":
      return "json";
    case "markdown":
      return "markdown";
    default:
      return "plaintext";
  }
}

/**
 * Validate YAML content
 */
function validateYaml(content: string): ValidationResult {
  if (!content.trim()) {
    return { valid: true };
  }

  try {
    yaml.load(content);
    return { valid: true };
  } catch (err) {
    if (err instanceof yaml.YAMLException) {
      return {
        valid: false,
        error: err.message,
        line: err.mark?.line === undefined ? undefined : err.mark.line + 1,
      };
    }
    return {
      valid: false,
      error: err instanceof Error ? err.message : "Invalid YAML",
    };
  }
}

/**
 * Monaco-based YAML editor with syntax highlighting and validation.
 */
export function YamlEditor({
  value,
  onChange,
  onSave,
  readOnly = false,
  language,
  fileType,
  className,
  loading = false,
}: YamlEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const monacoLanguage = language || getLanguage(fileType);
  const isYaml = monacoLanguage === "yaml";

  // Validate YAML - computed synchronously (debounce happens on input side)
  const validation = useMemo((): ValidationResult => {
    if (!isYaml) {
      return { valid: true };
    }
    return validateYaml(value);
  }, [value, isYaml]);

  const handleEditorDidMount: OnMount = useCallback(
    (editor, monaco) => {
      editorRef.current = editor;

      // Configure editor keybindings
      // Ctrl/Cmd+S to save
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
        onSave?.();
      });

      // Configure YAML-specific settings
      if (isYaml) {
        monaco.languages.setLanguageConfiguration("yaml", {
          comments: { lineComment: "#" },
          brackets: [
            ["{", "}"],
            ["[", "]"],
          ],
          autoClosingPairs: [
            { open: "{", close: "}" },
            { open: "[", close: "]" },
            { open: '"', close: '"' },
            { open: "'", close: "'" },
          ],
          indentationRules: {
            increaseIndentPattern: /^.*:\s*$/,
            decreaseIndentPattern: /^\s*$/,
          },
        });
      }
    },
    [onSave, isYaml]
  );

  const handleChange: OnChange = useCallback(
    (newValue) => {
      onChange?.(newValue || "");
    },
    [onChange]
  );

  if (loading) {
    return (
      <div className={cn("flex items-center justify-center h-full", className)}>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-sm text-muted-foreground">Loading file...</span>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col h-full", className)}>
      {/* Editor */}
      <div className="flex-1 min-h-0">
        <Editor
          height="100%"
          language={monacoLanguage}
          value={value}
          onChange={handleChange}
          onMount={handleEditorDidMount}
          theme="vs-dark"
          options={{
            readOnly,
            minimap: { enabled: false },
            fontSize: 13,
            lineNumbers: "on",
            wordWrap: "on",
            scrollBeyondLastLine: false,
            automaticLayout: true,
            tabSize: 2,
            insertSpaces: true,
            formatOnPaste: true,
            formatOnType: true,
            folding: true,
            foldingStrategy: "indentation",
            scrollbar: {
              verticalScrollbarSize: 10,
              horizontalScrollbarSize: 10,
            },
            padding: { top: 8, bottom: 8 },
          }}
          loading={
            <div className="flex items-center justify-center h-full">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          }
        />
      </div>

      {/* Validation status bar (only for YAML) */}
      {isYaml && (
        <div className="flex items-center gap-2 px-3 py-1.5 border-t bg-muted/30 text-xs">
          {validation.valid ? (
            <>
              <CheckCircle className="h-3.5 w-3.5 text-green-500" />
              <span className="text-muted-foreground">Valid YAML</span>
            </>
          ) : (
            <>
              <AlertTriangle className="h-3.5 w-3.5 text-amber-500" />
              <span className="text-amber-500 truncate">
                {validation.line !== undefined && `Line ${validation.line}: `}
                {validation.error}
              </span>
            </>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Empty state when no file is selected
 */
export function YamlEditorEmptyState() {
  return (
    <div className="flex items-center justify-center h-full text-muted-foreground bg-muted/10">
      <div className="text-center">
        <p className="text-sm">No file selected</p>
        <p className="text-xs mt-1">Select a file from the tree to view and edit</p>
      </div>
    </div>
  );
}
