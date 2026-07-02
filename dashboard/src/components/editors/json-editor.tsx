/**
 * JsonEditor — a compact Monaco-based JSON input with self-run validation.
 *
 * Shared by the function Test tab and the ToolRegistry Test tab. Replaces a
 * raw <textarea> so hand-editing multi-line JSON gets real editor affordances:
 * syntax highlighting, bracket matching/auto-closing, and format-on-paste/type.
 *
 * The bundled Monaco is @codingame/monaco-vscode-editor-api, which doesn't ship
 * the standalone JSON language service (schema diagnostics + worker), so — like
 * the arena YamlEditor — validation is computed here with JSON.parse and shown
 * in a status bar rather than via a language worker.
 *
 * Browser-only (Monaco touches window/Worker); excluded from unit coverage and
 * exercised via E2E, with the toggle/parse logic tested in the panels through a
 * lightweight mock of this module.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import Editor, { type OnChange, type OnMount, type BeforeMount } from "@monaco-editor/react";
import { useTheme } from "next-themes";
import { AlertTriangle, CheckCircle } from "lucide-react";
import { cn } from "@/lib/utils";

const THEME_LIGHT = "omnia-json-light";
const THEME_DARK = "omnia-json-dark";

/**
 * Register themes that inherit vs / vs-dark but drop the editor background to
 * transparent, so the editor adopts the surrounding card's themed background
 * instead of Monaco's stock white / #1e1e1e.
 */
const defineThemes: BeforeMount = (monaco) => {
  const transparent = { "editor.background": "#00000000", "editorGutter.background": "#00000000" };
  monaco.editor.defineTheme(THEME_LIGHT, {
    base: "vs",
    inherit: true,
    rules: [],
    colors: transparent,
  });
  monaco.editor.defineTheme(THEME_DARK, {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: transparent,
  });
};

interface JsonEditorProps {
  value: string;
  onChange: (value: string) => void;
  /** Accessible label for the editor surface. */
  ariaLabel?: string;
  readOnly?: boolean;
  /** Editor body height in pixels (excludes the status bar). */
  height?: number;
  className?: string;
  /** Invoked on Cmd/Ctrl+Enter inside the editor (e.g. to run a function). */
  onSubmit?: () => void;
}

interface JsonValidation {
  valid: boolean;
  error?: string;
}

/** Validate JSON content; empty is treated as valid (nothing to run yet). */
function validateJson(content: string): JsonValidation {
  if (!content.trim()) return { valid: true };
  try {
    JSON.parse(content);
    return { valid: true };
  } catch (e) {
    return { valid: false, error: e instanceof Error ? e.message : "Invalid JSON" };
  }
}

export function JsonEditor({
  value,
  onChange,
  ariaLabel,
  readOnly = false,
  height = 160,
  className,
  onSubmit,
}: Readonly<JsonEditorProps>) {
  const { resolvedTheme } = useTheme();
  const theme = resolvedTheme === "light" ? THEME_LIGHT : THEME_DARK;
  const validation = useMemo(() => validateJson(value), [value]);
  const handleChange: OnChange = useCallback((v) => onChange(v ?? ""), [onChange]);

  // Latest-ref so the Monaco command (bound once at mount) always calls the
  // current onSubmit rather than a stale closure.
  const onSubmitRef = useRef(onSubmit);
  useEffect(() => {
    onSubmitRef.current = onSubmit;
  }, [onSubmit]);

  const handleMount: OnMount = useCallback((editor, monaco) => {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      onSubmitRef.current?.();
    });
  }, []);

  return (
    <div className={cn("rounded-md border overflow-hidden bg-muted/30", className)}>
      <Editor
        height={height}
        language="json"
        value={value}
        onChange={handleChange}
        beforeMount={defineThemes}
        onMount={handleMount}
        theme={theme}
        options={{
          readOnly,
          ariaLabel,
          minimap: { enabled: false },
          fontSize: 13,
          lineNumbers: "off",
          wordWrap: "on",
          scrollBeyondLastLine: false,
          automaticLayout: true,
          tabSize: 2,
          insertSpaces: true,
          formatOnPaste: true,
          formatOnType: true,
          folding: false,
          renderLineHighlight: "none",
          overviewRulerLanes: 0,
          scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
          padding: { top: 6, bottom: 6 },
        }}
      />
      <div className="flex items-center gap-2 px-3 py-1 border-t bg-muted/30 text-xs">
        {validation.valid ? (
          <>
            <CheckCircle className="h-3.5 w-3.5 text-success" />
            <span className="text-muted-foreground">Valid JSON</span>
          </>
        ) : (
          <>
            <AlertTriangle className="h-3.5 w-3.5 text-warning shrink-0" />
            <span className="text-warning truncate">{validation.error}</span>
          </>
        )}
      </div>
    </div>
  );
}
