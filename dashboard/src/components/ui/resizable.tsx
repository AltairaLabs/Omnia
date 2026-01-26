"use client";

import * as React from "react";
import { cn } from "@/lib/utils";
import { GripVertical } from "lucide-react";

// =============================================================================
// Context
// =============================================================================

interface ResizableContextValue {
  direction: "horizontal" | "vertical";
}

const ResizableContext = React.createContext<ResizableContextValue | null>(null);

function useResizableContext() {
  const context = React.useContext(ResizableContext);
  if (!context) {
    throw new Error("Resizable components must be used within a ResizablePanelGroup");
  }
  return context;
}

// =============================================================================
// ResizablePanelGroup
// =============================================================================

interface ResizablePanelGroupProps {
  direction: "horizontal" | "vertical";
  children: React.ReactNode;
  className?: string;
}

export function ResizablePanelGroup({
  direction,
  children,
  className,
}: ResizablePanelGroupProps) {
  const contextValue = React.useMemo(
    () => ({ direction }),
    [direction]
  );

  return (
    <ResizableContext.Provider value={contextValue}>
      <div
        className={cn(
          "flex h-full w-full",
          direction === "horizontal" ? "flex-row" : "flex-col",
          className
        )}
      >
        {children}
      </div>
    </ResizableContext.Provider>
  );
}

// =============================================================================
// ResizablePanel
// =============================================================================

interface ResizablePanelProps {
  children: React.ReactNode;
  defaultSize?: number;
  minSize?: number;
  maxSize?: number;
  className?: string;
}

export function ResizablePanel({
  children,
  defaultSize = 50,
  minSize = 10,
  maxSize = 90,
  className,
}: ResizablePanelProps) {
  const { direction } = useResizableContext();

  return (
    <div
      className={cn("overflow-hidden", className)}
      style={{
        [direction === "horizontal" ? "width" : "height"]: `${defaultSize}%`,
        [direction === "horizontal" ? "minWidth" : "minHeight"]: `${minSize}%`,
        [direction === "horizontal" ? "maxWidth" : "maxHeight"]: `${maxSize}%`,
        flexShrink: 0,
        flexGrow: 0,
      }}
    >
      {children}
    </div>
  );
}

// =============================================================================
// ResizableHandle
// =============================================================================

interface ResizableHandleProps {
  withHandle?: boolean;
  className?: string;
}

export function ResizableHandle({ withHandle, className }: ResizableHandleProps) {
  const { direction } = useResizableContext();

  return (
    <div
      className={cn(
        "relative flex items-center justify-center",
        "bg-border",
        direction === "horizontal"
          ? "w-1 cursor-col-resize"
          : "h-1 cursor-row-resize",
        className
      )}
    >
      {withHandle && (
        <div
          className={cn(
            "absolute z-10 flex items-center justify-center",
            "rounded-sm border bg-border",
            direction === "horizontal"
              ? "h-6 w-3 -translate-x-1/2 left-1/2"
              : "w-6 h-3 -translate-y-1/2 top-1/2"
          )}
        >
          <GripVertical
            className={cn(
              "h-3 w-3 text-muted-foreground",
              direction === "vertical" && "rotate-90"
            )}
          />
        </div>
      )}
    </div>
  );
}
