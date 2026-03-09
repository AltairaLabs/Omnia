"use client";

import { useMemo, useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import { ChevronRight, ChevronDown } from "lucide-react";

interface JsonBlockProps {
  data: unknown;
  className?: string;
  /** Keys to collapse by default */
  defaultCollapsed?: string[];
  /** Maximum depth to expand by default (0 = collapse all, undefined = expand all) */
  defaultExpandDepth?: number;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isArray(value: unknown): value is unknown[] {
  return Array.isArray(value);
}

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
    return (
      <span className="text-emerald-600 dark:text-emerald-400">
        &quot;{value}&quot;
      </span>
    );
  }
  return <span>{String(value)}</span>;
}

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

interface JsonNodeProps {
  keyName?: string;
  value: unknown;
  depth: number;
  isLast: boolean;
  collapsedState: Set<string>;
  toggleCollapse: (path: string) => void;
  path: string;
}

function JsonNode({
  keyName,
  value,
  depth,
  isLast,
  collapsedState,
  toggleCollapse,
  path,
}: JsonNodeProps) {
  const indent = "  ".repeat(depth);
  const childIndent = "  ".repeat(depth + 1);
  const isCollapsible = isObject(value) || isArray(value);
  const isCollapsed = collapsedState.has(path);
  const comma = isLast ? "" : ",";

  const keyPrefix = keyName !== undefined ? (
    <>
      <span className="text-violet-600 dark:text-violet-400">
        &quot;{keyName}&quot;
      </span>
      <span className="text-foreground">: </span>
    </>
  ) : null;

  // Primitive values
  if (!isCollapsible) {
    return (
      <div className="leading-relaxed">
        {indent}{keyPrefix}{formatValue(value)}{comma}
      </div>
    );
  }

  const openBracket = isArray(value) ? "[" : "{";
  const closeBracket = isArray(value) ? "]" : "}";
  const entries = isArray(value)
    ? (value as unknown[]).map((v, i) => ({ key: undefined, value: v, path: `${path}[${i}]` }))
    : Object.entries(value as Record<string, unknown>).map(([k, v]) => ({ key: k, value: v, path: `${path}.${k}` }));

  // Empty object/array
  if (entries.length === 0) {
    return (
      <div className="leading-relaxed">
        {indent}{keyPrefix}
        <span className="text-muted-foreground">{openBracket}{closeBracket}</span>
        {comma}
      </div>
    );
  }

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    toggleCollapse(path);
  };

  // Collapsed view
  if (isCollapsed) {
    return (
      <div className="leading-relaxed">
        {indent}{keyPrefix}
        <span
          className="cursor-pointer hover:bg-muted-foreground/10 rounded -mx-0.5 px-0.5 inline-flex items-center"
          onClick={handleClick}
        >
          <ChevronRight className="h-3 w-3 mr-0.5 text-muted-foreground inline" />
          <span className="text-foreground">{openBracket}</span>
          <span className="text-muted-foreground text-xs mx-1">
            {countItems(value)}
          </span>
          <span className="text-foreground">{closeBracket}</span>
        </span>
        {comma}
      </div>
    );
  }

  // Expanded view
  return (
    <div>
      <div className="leading-relaxed">
        {indent}{keyPrefix}
        <span
          className="cursor-pointer hover:bg-muted-foreground/10 rounded -mx-0.5 px-0.5 inline-flex items-center"
          onClick={handleClick}
        >
          <ChevronDown className="h-3 w-3 mr-0.5 text-muted-foreground inline" />
          <span className="text-foreground">{openBracket}</span>
        </span>
      </div>
      {entries.map((entry, i) => (
        <JsonNode
          key={entry.path}
          keyName={entry.key}
          value={entry.value}
          depth={depth + 1}
          isLast={i === entries.length - 1}
          collapsedState={collapsedState}
          toggleCollapse={toggleCollapse}
          path={entry.path}
        />
      ))}
      <div className="leading-relaxed">
        {childIndent.slice(2)}{closeBracket}{comma}
      </div>
    </div>
  );
}

function buildInitialCollapsedState(
  data: unknown,
  defaultCollapsed: string[],
  defaultExpandDepth: number | undefined,
  path: string = "root",
  depth: number = 0,
): Set<string> {
  const collapsed = new Set<string>();

  function traverse(value: unknown, currentPath: string, currentDepth: number, key?: string) {
    if (!isObject(value) && !isArray(value)) return;

    const shouldCollapse =
      (key !== undefined && defaultCollapsed.includes(key)) ||
      (defaultExpandDepth !== undefined && currentDepth >= defaultExpandDepth);

    if (shouldCollapse) {
      collapsed.add(currentPath);
      return;
    }

    if (isObject(value)) {
      Object.entries(value).forEach(([k, v]) => {
        traverse(v, `${currentPath}.${k}`, currentDepth + 1, k);
      });
    } else if (isArray(value)) {
      value.forEach((item, index) => {
        traverse(item, `${currentPath}[${index}]`, currentDepth + 1);
      });
    }
  }

  traverse(data, path, depth);
  return collapsed;
}

export function JsonBlock({
  data,
  className,
  defaultCollapsed = [],
  defaultExpandDepth,
}: Readonly<JsonBlockProps>) {
  const initialCollapsed = useMemo(
    () => buildInitialCollapsedState(data, defaultCollapsed, defaultExpandDepth),
    [data, defaultCollapsed, defaultExpandDepth]
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

  // Primitive data
  if (!isObject(data) && !isArray(data)) {
    return (
      <pre
        className={cn(
          "bg-muted/50 p-2 rounded overflow-auto text-xs font-mono",
          className
        )}
        data-testid="json-block"
      >
        <code>{formatValue(data)}</code>
      </pre>
    );
  }

  return (
    <pre
      className={cn(
        "bg-muted/50 p-2 rounded overflow-auto text-xs font-mono",
        className
      )}
      data-testid="json-block"
    >
      <code>
        <JsonNode
          value={data}
          depth={0}
          isLast
          collapsedState={collapsedState}
          toggleCollapse={toggleCollapse}
          path="root"
        />
      </code>
    </pre>
  );
}
