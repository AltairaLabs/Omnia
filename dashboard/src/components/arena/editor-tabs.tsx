"use client";

import { useCallback } from "react";
import { X, Circle } from "lucide-react";
import { cn } from "@/lib/utils";
import { useProjectEditorStore } from "@/stores";
import type { OpenFile } from "@/types/arena-project";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface EditorTabsProps {
  readonly className?: string;
}

/**
 * Tab bar for open files in the editor.
 * Shows dirty indicator, close button, and supports tab overflow scrolling.
 */
export function EditorTabs({ className }: EditorTabsProps) {
  const openFiles = useProjectEditorStore((state) => state.openFiles);
  const activeFilePath = useProjectEditorStore((state) => state.activeFilePath);
  const setActiveFile = useProjectEditorStore((state) => state.setActiveFile);
  const closeFile = useProjectEditorStore((state) => state.closeFile);

  const handleTabClick = useCallback(
    (path: string) => {
      setActiveFile(path);
    },
    [setActiveFile]
  );

  const handleCloseTab = useCallback(
    (e: React.MouseEvent, path: string) => {
      e.stopPropagation();
      closeFile(path);
    },
    [closeFile]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, path: string) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        setActiveFile(path);
      }
    },
    [setActiveFile]
  );

  if (openFiles.length === 0) {
    return null;
  }

  return (
    <TooltipProvider>
      <div className={cn("border-b bg-muted/30", className)}>
        <ScrollArea className="w-full">
          <div className="flex items-center p-1 gap-0.5">
            {openFiles.map((file) => (
              <EditorTab
                key={file.path}
                file={file}
                isActive={activeFilePath === file.path}
                onTabClick={handleTabClick}
                onCloseTab={handleCloseTab}
                onKeyDown={handleKeyDown}
              />
            ))}
          </div>
          <ScrollBar orientation="horizontal" className="h-2" />
        </ScrollArea>
      </div>
    </TooltipProvider>
  );
}

interface EditorTabProps {
  file: OpenFile;
  isActive: boolean;
  onTabClick: (path: string) => void;
  onCloseTab: (e: React.MouseEvent, path: string) => void;
  onKeyDown: (e: React.KeyboardEvent, path: string) => void;
}

function EditorTab({
  file,
  isActive,
  onTabClick,
  onCloseTab,
  onKeyDown,
}: EditorTabProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          role="tab"
          tabIndex={0}
          onClick={() => onTabClick(file.path)}
          onKeyDown={(e) => onKeyDown(e, file.path)}
          className={cn(
            "group flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-t-md transition-colors cursor-pointer",
            "hover:bg-background/80 border-b-2",
            isActive
              ? "bg-background text-foreground border-primary shadow-sm"
              : "text-muted-foreground border-transparent"
          )}
          aria-selected={isActive}
        >
          {/* Dirty indicator */}
          {file.isDirty && (
            <Circle className="h-2 w-2 fill-current text-amber-500 flex-shrink-0" />
          )}

          {/* File name */}
          <span className="truncate max-w-[150px]">{file.name}</span>

          {/* Close button */}
          <button
            type="button"
            onClick={(e) => onCloseTab(e, file.path)}
            className={cn(
              "ml-1 p-0.5 rounded hover:bg-muted",
              "opacity-0 group-hover:opacity-100 transition-opacity",
              isActive && "opacity-100"
            )}
            aria-label={`Close ${file.name}`}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      </TooltipTrigger>
      <TooltipContent side="bottom" className="max-w-xs">
        <p className="font-mono text-xs">{file.path}</p>
        {file.isDirty && (
          <p className="text-amber-500 text-xs mt-1">Unsaved changes</p>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Empty state when no files are open
 */
export function EditorTabsEmptyState() {
  return (
    <div className="flex items-center justify-center h-full text-muted-foreground">
      <div className="text-center">
        <p className="text-sm">No files open</p>
        <p className="text-xs mt-1">Select a file from the tree to start editing</p>
      </div>
    </div>
  );
}
