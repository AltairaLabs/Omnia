"use client";

import { cn } from "@/lib/utils";
import { Code2, Workflow } from "lucide-react";

export type EditorView = "yaml" | "workload";

export function EditorViewToggle({
  view,
  onChange,
}: Readonly<{ view: EditorView; onChange: (v: EditorView) => void }>) {
  const btn = (v: EditorView, label: string, Icon: typeof Code2) => (
    <button
      type="button"
      aria-pressed={view === v}
      onClick={() => onChange(v)}
      className={cn(
        "inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded transition-colors",
        view === v ? "bg-background shadow-sm text-foreground" : "text-muted-foreground hover:text-foreground",
      )}
    >
      <Icon className="h-3.5 w-3.5" />
      {label}
    </button>
  );
  return (
    <div className="inline-flex items-center gap-0.5 rounded-md border bg-muted/40 p-0.5">
      {btn("yaml", "YAML", Code2)}
      {btn("workload", "Workload", Workflow)}
    </div>
  );
}
