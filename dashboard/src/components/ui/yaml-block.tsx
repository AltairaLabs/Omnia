"use client";

import { useMemo, useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import { ChevronRight, ChevronDown } from "lucide-react";

interface YamlBlockProps {
  data: unknown;
  className?: string;
  /** Keys to collapse by default */
  defaultCollapsed?: string[];
}

// System keys that should be collapsed by default
const DEFAULT_COLLAPSED_KEYS = [
  "managedFields",
  "status",
  "ownerReferences",
  "finalizers",
];

// Check if a key should be collapsed by default
function shouldCollapseByDefault(key: string, defaultCollapsed: string[]): boolean {
  return defaultCollapsed.includes(key);
}

// Check if value is an object (not null, not array)
function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Check if value is an array
function isArray(value: unknown): value is unknown[] {
  return Array.isArray(value);
}

// Format a primitive value with syntax highlighting
function formatValue(value: unknown): React.ReactNode {
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic">null</span>;
  }
  if (typeof value === "boolean") {
    return (
      <span className="text-orange-500 dark:text-orange-400">
        {value.toString()}
      </span>
    );
  }
  if (typeof value === "number") {
    return (
      <span className="text-cyan-600 dark:text-cyan-400">{value}</span>
    );
  }
  if (typeof value === "string") {
    // Check if it needs quoting
    const needsQuotes =
      value.includes(":") ||
      value.includes("#") ||
      value.includes("\n") ||
      value.startsWith(" ") ||
      value.endsWith(" ") ||
      value === "";

    if (needsQuotes) {
      return (
        <span className="text-emerald-600 dark:text-emerald-400">
          &quot;{value}&quot;
        </span>
      );
    }
    return (
      <span className="text-green-600 dark:text-green-400">{value}</span>
    );
  }
  return <span>{String(value)}</span>;
}

// Count items in an object or array for the collapsed preview
function countItems(value: unknown): string {
  if (isArray(value)) {
    return `${value.length} item${value.length !== 1 ? "s" : ""}`;
  }
  if (isObject(value)) {
    const keys = Object.keys(value);
    return `${keys.length} key${keys.length !== 1 ? "s" : ""}`;
  }
  return "";
}

interface YamlNodeProps {
  keyName?: string;
  value: unknown;
  depth: number;
  isArrayItem?: boolean;
  collapsedState: Set<string>;
  toggleCollapse: (path: string) => void;
  path: string;
  defaultCollapsed: string[];
}

function YamlNode({
  keyName,
  value,
  depth,
  isArrayItem,
  collapsedState,
  toggleCollapse,
  path,
  defaultCollapsed,
}: YamlNodeProps) {
  const indent = "  ".repeat(depth);
  const isCollapsible = isObject(value) || isArray(value);
  const isCollapsed = collapsedState.has(path);

  // Handle primitive values
  if (!isCollapsible) {
    if (isArrayItem) {
      return (
        <div className="leading-relaxed">
          {indent}
          <span className="text-foreground">- </span>
          {formatValue(value)}
        </div>
      );
    }
    return (
      <div className="leading-relaxed">
        {indent}
        <span className="text-violet-600 dark:text-violet-400">{keyName}</span>
        <span className="text-foreground">: </span>
        {formatValue(value)}
      </div>
    );
  }

  // Handle collapsible objects/arrays
  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    toggleCollapse(path);
  };

  const ChevronIcon = isCollapsed ? ChevronRight : ChevronDown;
  const isEmpty = isArray(value) ? value.length === 0 : Object.keys(value as Record<string, unknown>).length === 0;

  // Empty object/array
  if (isEmpty) {
    if (isArrayItem) {
      return (
        <div className="leading-relaxed">
          {indent}
          <span className="text-foreground">- </span>
          <span className="text-muted-foreground">{isArray(value) ? "[]" : "{}"}</span>
        </div>
      );
    }
    return (
      <div className="leading-relaxed">
        {indent}
        <span className="text-violet-600 dark:text-violet-400">{keyName}</span>
        <span className="text-foreground">: </span>
        <span className="text-muted-foreground">{isArray(value) ? "[]" : "{}"}</span>
      </div>
    );
  }

  // Collapsible header
  const header = (
    <div
      className="leading-relaxed cursor-pointer hover:bg-muted-foreground/10 rounded -mx-1 px-1 inline-flex items-center"
      onClick={handleClick}
    >
      <ChevronIcon className="h-3 w-3 mr-1 text-muted-foreground flex-shrink-0" />
      {isArrayItem ? (
        <>
          <span className="text-foreground">- </span>
          {isCollapsed && (
            <span className="text-muted-foreground text-xs ml-1">
              ({countItems(value)})
            </span>
          )}
        </>
      ) : (
        <>
          <span className="text-violet-600 dark:text-violet-400">{keyName}</span>
          <span className="text-foreground">:</span>
          {isCollapsed && (
            <span className="text-muted-foreground text-xs ml-1">
              ({countItems(value)})
            </span>
          )}
        </>
      )}
    </div>
  );

  if (isCollapsed) {
    return <div>{indent.slice(2)}{header}</div>;
  }

  // Expanded content
  if (isArray(value)) {
    return (
      <div>
        <div>{indent.slice(2)}{header}</div>
        {(value as unknown[]).map((item, index) => (
          <YamlNode
            key={`${path}[${index}]`}
            value={item}
            depth={depth + 1}
            isArrayItem
            collapsedState={collapsedState}
            toggleCollapse={toggleCollapse}
            path={`${path}[${index}]`}
            defaultCollapsed={defaultCollapsed}
          />
        ))}
      </div>
    );
  }

  // Object
  const obj = value as Record<string, unknown>;
  return (
    <div>
      <div>{indent.slice(2)}{header}</div>
      {Object.entries(obj).map(([key, val]) => (
        <YamlNode
          key={`${path}.${key}`}
          keyName={key}
          value={val}
          depth={depth + 1}
          collapsedState={collapsedState}
          toggleCollapse={toggleCollapse}
          path={`${path}.${key}`}
          defaultCollapsed={defaultCollapsed}
        />
      ))}
    </div>
  );
}

// Build initial collapsed state from data
function buildInitialCollapsedState(
  data: unknown,
  defaultCollapsed: string[],
  path: string = "root"
): Set<string> {
  const collapsed = new Set<string>();

  function traverse(value: unknown, currentPath: string, key?: string) {
    if (key && shouldCollapseByDefault(key, defaultCollapsed)) {
      collapsed.add(currentPath);
      return; // Don't traverse children of collapsed nodes
    }

    if (isObject(value)) {
      Object.entries(value).forEach(([k, v]) => {
        traverse(v, `${currentPath}.${k}`, k);
      });
    } else if (isArray(value)) {
      value.forEach((item, index) => {
        traverse(item, `${currentPath}[${index}]`);
      });
    }
  }

  traverse(data, path);
  return collapsed;
}

export function YamlBlock({
  data,
  className,
  defaultCollapsed = DEFAULT_COLLAPSED_KEYS,
}: Readonly<YamlBlockProps>) {
  const initialCollapsed = useMemo(
    () => buildInitialCollapsedState(data, defaultCollapsed),
    [data, defaultCollapsed]
  );

  const [collapsedState, setCollapsedState] = useState<Set<string>>(initialCollapsed);

  const toggleCollapse = useCallback((path: string) => {
    setCollapsedState((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  // Handle null/undefined/primitive data
  if (!isObject(data) && !isArray(data)) {
    return (
      <pre
        className={cn(
          "bg-muted p-4 rounded-lg overflow-auto text-sm font-mono",
          className
        )}
      >
        <code>{formatValue(data)}</code>
      </pre>
    );
  }

  return (
    <pre
      className={cn(
        "bg-muted p-4 rounded-lg overflow-auto text-sm font-mono",
        className
      )}
    >
      <code>
        {isArray(data) ? (
          (data as unknown[]).map((item, index) => (
            <YamlNode
              key={`root[${index}]`}
              value={item}
              depth={0}
              isArrayItem
              collapsedState={collapsedState}
              toggleCollapse={toggleCollapse}
              path={`root[${index}]`}
              defaultCollapsed={defaultCollapsed}
            />
          ))
        ) : (
          Object.entries(data as Record<string, unknown>).map(([key, value]) => (
            <YamlNode
              key={`root.${key}`}
              keyName={key}
              value={value}
              depth={0}
              collapsedState={collapsedState}
              toggleCollapse={toggleCollapse}
              path={`root.${key}`}
              defaultCollapsed={defaultCollapsed}
            />
          ))
        )}
      </code>
    </pre>
  );
}
