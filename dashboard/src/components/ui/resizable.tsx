"use client";

import * as React from "react";
import { cn } from "@/lib/utils";
import { GripVertical } from "lucide-react";

// =============================================================================
// Context
// =============================================================================

interface ResizableContextValue {
  orientation: "horizontal" | "vertical";
  groupRef: React.RefObject<HTMLDivElement | null>;
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
  orientation: "horizontal" | "vertical";
  children: React.ReactNode;
  className?: string;
}

export function ResizablePanelGroup({
  orientation,
  children,
  className,
}: ResizablePanelGroupProps) {
  const groupRef = React.useRef<HTMLDivElement>(null);

  const contextValue = React.useMemo(
    () => ({ orientation, groupRef }),
    [orientation]
  );

  return (
    <ResizableContext.Provider value={contextValue}>
      <div
        ref={groupRef}
        className={cn(
          "flex h-full w-full",
          orientation === "horizontal" ? "flex-row" : "flex-col",
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
  className,
}: ResizablePanelProps) {
  const { orientation } = useResizableContext();

  return (
    <div
      data-resizable-panel=""
      data-min-size={minSize}
      className={cn("overflow-hidden", className)}
      style={{
        flex: `${defaultSize} 1 0%`,
        minWidth: orientation === "horizontal" ? `${minSize}%` : undefined,
        minHeight: orientation === "vertical" ? `${minSize}%` : undefined,
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
  const { orientation, groupRef } = useResizableContext();
  const handleRef = React.useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = React.useState(false);

  const handleMouseDown = React.useCallback((e: React.MouseEvent) => {
    e.preventDefault();

    const handle = handleRef.current;
    const group = groupRef.current;
    if (!handle || !group) return;

    // Find adjacent panels by looking at siblings
    const prevPanel = handle.previousElementSibling as HTMLElement | null;
    const nextPanel = handle.nextElementSibling as HTMLElement | null;

    if (!prevPanel?.hasAttribute("data-resizable-panel") ||
        !nextPanel?.hasAttribute("data-resizable-panel")) {
      return;
    }

    setIsDragging(true);

    const isHorizontal = orientation === "horizontal";
    const prop = isHorizontal ? "width" : "height";
    const clientProp = isHorizontal ? "clientX" : "clientY";

    let lastPos = e[clientProp];

    // Get initial sizes
    const groupSize = group.getBoundingClientRect()[prop];
    let prevSize = prevPanel.getBoundingClientRect()[prop];
    let nextSize = nextPanel.getBoundingClientRect()[prop];

    // Get min sizes (as percentage, convert to pixels)
    const prevMinPct = parseFloat(prevPanel.dataset.minSize || "10");
    const nextMinPct = parseFloat(nextPanel.dataset.minSize || "10");
    const prevMin = (prevMinPct / 100) * groupSize;
    const nextMin = (nextMinPct / 100) * groupSize;

    const handleMouseMove = (moveEvent: MouseEvent) => {
      const currentPos = moveEvent[clientProp];
      const delta = currentPos - lastPos;
      lastPos = currentPos;

      let newPrevSize = prevSize + delta;
      let newNextSize = nextSize - delta;

      // Enforce minimums
      if (newPrevSize < prevMin) {
        newPrevSize = prevMin;
        newNextSize = prevSize + nextSize - prevMin;
      }
      if (newNextSize < nextMin) {
        newNextSize = nextMin;
        newPrevSize = prevSize + nextSize - nextMin;
      }

      prevSize = newPrevSize;
      nextSize = newNextSize;

      // Convert to flex-basis
      prevPanel.style.flex = `0 0 ${newPrevSize}px`;
      nextPanel.style.flex = `0 0 ${newNextSize}px`;
    };

    const handleMouseUp = () => {
      setIsDragging(false);
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    document.body.style.cursor = isHorizontal ? "col-resize" : "row-resize";
    document.body.style.userSelect = "none";
  }, [orientation, groupRef]);

  return (
    <div
      ref={handleRef}
      onMouseDown={handleMouseDown}
      className={cn(
        "relative flex items-center justify-center bg-border shrink-0",
        orientation === "horizontal"
          ? "w-1 cursor-col-resize hover:bg-primary/20"
          : "h-1 cursor-row-resize hover:bg-primary/20",
        isDragging && "bg-primary/30",
        className
      )}
    >
      {withHandle && (
        <div
          className={cn(
            "absolute z-10 flex items-center justify-center rounded-sm border bg-border",
            orientation === "horizontal"
              ? "h-6 w-3"
              : "w-6 h-3"
          )}
        >
          <GripVertical
            className={cn(
              "h-3 w-3 text-muted-foreground",
              orientation === "vertical" && "rotate-90"
            )}
          />
        </div>
      )}
    </div>
  );
}
