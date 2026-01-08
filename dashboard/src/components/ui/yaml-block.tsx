"use client";

import { useMemo } from "react";
import yaml from "js-yaml";
import { cn } from "@/lib/utils";

interface YamlBlockProps {
  data: unknown;
  className?: string;
}

// Simple YAML syntax highlighting using regex
function highlightYaml(yamlString: string): React.ReactNode[] {
  const lines = yamlString.split("\n");

  return lines.map((line, index) => {
    // Empty line
    if (!line.trim()) {
      return <span key={`line-${index}`}>{"\n"}</span>;
    }

    // Comment
    if (line.trim().startsWith("#")) {
      return (
        <span key={`line-${index}`}>
          <span className="text-muted-foreground italic">{line}</span>
          {"\n"}
        </span>
      );
    }

    // Key-value pair - use non-greedy match with explicit character class
    const keyMatch = line.match(/^(\s*)([a-zA-Z0-9_.-]+):\s*(.*)/);
    if (keyMatch) {
      const [, indent, key, rest] = keyMatch;
      const value = rest.trim();

      // Check value types for coloring
      let valueElement: React.ReactNode = null;

      if (value) {
        if (value === "true" || value === "false") {
          // Boolean
          valueElement = (
            <span className="text-orange-500 dark:text-orange-400">{value}</span>
          );
        } else if (/^-?\d+(\.\d+)?$/.test(value)) {
          // Number
          valueElement = (
            <span className="text-cyan-600 dark:text-cyan-400">{value}</span>
          );
        } else if (value.startsWith('"') || value.startsWith("'")) {
          // Quoted string - use slightly different shade to distinguish
          valueElement = (
            <span className="text-emerald-600 dark:text-emerald-400">{value}</span>
          );
        } else if (value === "null" || value === "~") {
          // Null
          valueElement = (
            <span className="text-muted-foreground italic">{value}</span>
          );
        } else {
          // Unquoted string
          valueElement = (
            <span className="text-green-600 dark:text-green-400">{value}</span>
          );
        }
      }

      return (
        <span key={`line-${index}`}>
          {indent}
          <span className="text-violet-600 dark:text-violet-400">{key}</span>
          <span className="text-foreground">:</span>
          {value ? " " : ""}
          {valueElement}
          {"\n"}
        </span>
      );
    }

    // Array item
    const arrayMatch = line.match(/^(\s*)-\s*(.*)/);
    if (arrayMatch) {
      const [, indent, value] = arrayMatch;
      return (
        <span key={`line-${index}`}>
          {indent}
          <span className="text-foreground">-</span>{" "}
          <span className="text-green-600 dark:text-green-400">{value}</span>
          {"\n"}
        </span>
      );
    }

    // Default
    return (
      <span key={`line-${index}`}>
        {line}
        {"\n"}
      </span>
    );
  });
}

export function YamlBlock({ data, className }: Readonly<YamlBlockProps>) {
  const yamlString = useMemo(() => {
    try {
      return yaml.dump(data, {
        indent: 2,
        lineWidth: -1,
        noRefs: true,
        sortKeys: false,
      });
    } catch {
      return "# Error converting to YAML";
    }
  }, [data]);

  const highlighted = useMemo(() => highlightYaml(yamlString), [yamlString]);

  return (
    <pre
      className={cn(
        "bg-muted p-4 rounded-lg overflow-auto text-sm font-mono",
        className
      )}
    >
      <code>{highlighted}</code>
    </pre>
  );
}
