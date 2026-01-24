"use client";

import { useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { Plus, X, ChevronDown, ChevronUp } from "lucide-react";

export type LabelMatchOperator = "In" | "NotIn" | "Exists" | "DoesNotExist";

export interface LabelMatchExpression {
  key: string;
  operator: LabelMatchOperator;
  values?: string[];
}

export interface LabelSelectorValue {
  matchLabels?: Record<string, string>;
  matchExpressions?: LabelMatchExpression[];
}

export interface K8sLabelSelectorProps {
  value: LabelSelectorValue;
  onChange: (value: LabelSelectorValue) => void;
  label?: string;
  description?: string;
  availableLabels?: Record<string, string[]>;
  previewComponent?: React.ReactNode;
  disabled?: boolean;
  className?: string;
}

interface MatchLabelEditorProps {
  matchLabels: Record<string, string>;
  onChange: (labels: Record<string, string>) => void;
  availableLabels?: Record<string, string[]>;
  disabled?: boolean;
}

function MatchLabelEditor({
  matchLabels,
  onChange,
  availableLabels,
  disabled,
}: MatchLabelEditorProps) {
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  const handleAdd = () => {
    if (newKey.trim() && newValue.trim()) {
      onChange({ ...matchLabels, [newKey.trim()]: newValue.trim() });
      setNewKey("");
      setNewValue("");
    }
  };

  const handleRemove = (key: string) => {
    const updated = { ...matchLabels };
    delete updated[key];
    onChange(updated);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAdd();
    }
  };

  const entries = Object.entries(matchLabels);
  const availableKeys = availableLabels ? Object.keys(availableLabels) : [];

  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-muted-foreground">Match Labels</div>

      {/* Existing labels */}
      {entries.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {entries.map(([key, value]) => (
            <Badge
              key={key}
              variant="secondary"
              className="flex items-center gap-1 pr-1"
            >
              <span className="font-mono text-xs">{key}={value}</span>
              {!disabled && (
                <button
                  type="button"
                  onClick={() => handleRemove(key)}
                  className="ml-1 hover:bg-muted rounded-sm p-0.5"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </Badge>
          ))}
        </div>
      )}

      {/* Add new label */}
      {!disabled && (
        <div className="flex items-center gap-2">
          {availableKeys.length > 0 ? (
            <Select value={newKey} onValueChange={setNewKey}>
              <SelectTrigger className="h-8 w-[140px]">
                <SelectValue placeholder="Key" />
              </SelectTrigger>
              <SelectContent>
                {availableKeys
                  .filter((k) => !(k in matchLabels))
                  .map((key) => (
                    <SelectItem key={key} value={key}>
                      {key}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              value={newKey}
              onChange={(e) => setNewKey(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Key"
              className="h-8 w-[140px]"
            />
          )}
          <span className="text-muted-foreground">=</span>
          {availableLabels && newKey && availableLabels[newKey]?.length > 0 ? (
            <Select value={newValue} onValueChange={setNewValue}>
              <SelectTrigger className="h-8 w-[140px]">
                <SelectValue placeholder="Value" />
              </SelectTrigger>
              <SelectContent>
                {availableLabels[newKey].map((val) => (
                  <SelectItem key={val} value={val}>
                    {val}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              value={newValue}
              onChange={(e) => setNewValue(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Value"
              className="h-8 w-[140px]"
            />
          )}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAdd}
            disabled={!newKey.trim() || !newValue.trim()}
            className="h-8"
          >
            <Plus className="h-3 w-3" />
          </Button>
        </div>
      )}
    </div>
  );
}

interface MatchExpressionEditorProps {
  expressions: LabelMatchExpression[];
  onChange: (expressions: LabelMatchExpression[]) => void;
  availableLabels?: Record<string, string[]>;
  disabled?: boolean;
}

function MatchExpressionEditor({
  expressions,
  onChange,
  availableLabels,
  disabled,
}: MatchExpressionEditorProps) {
  const [newKey, setNewKey] = useState("");
  const [newOperator, setNewOperator] = useState<LabelMatchOperator>("In");
  const [newValues, setNewValues] = useState("");

  const operatorRequiresValues = (op: LabelMatchOperator) => op === "In" || op === "NotIn";

  const handleAdd = () => {
    if (!newKey.trim()) return;
    if (operatorRequiresValues(newOperator) && !newValues.trim()) return;

    const expression: LabelMatchExpression = {
      key: newKey.trim(),
      operator: newOperator,
    };

    if (operatorRequiresValues(newOperator)) {
      expression.values = newValues
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean);
    }

    onChange([...expressions, expression]);
    setNewKey("");
    setNewOperator("In");
    setNewValues("");
  };

  const handleRemove = (index: number) => {
    onChange(expressions.filter((_, i) => i !== index));
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAdd();
    }
  };

  const formatExpression = (expr: LabelMatchExpression) => {
    switch (expr.operator) {
      case "In":
        return `${expr.key} in (${expr.values?.join(", ") || ""})`;
      case "NotIn":
        return `${expr.key} notin (${expr.values?.join(", ") || ""})`;
      case "Exists":
        return expr.key;
      case "DoesNotExist":
        return `!${expr.key}`;
    }
  };

  const availableKeys = availableLabels ? Object.keys(availableLabels) : [];

  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-muted-foreground">Match Expressions</div>

      {/* Existing expressions */}
      {expressions.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {expressions.map((expr, index) => (
            <Badge
              key={`${expr.key}-${index}`}
              variant="outline"
              className="flex items-center gap-1 pr-1"
            >
              <span className="font-mono text-xs">{formatExpression(expr)}</span>
              {!disabled && (
                <button
                  type="button"
                  onClick={() => handleRemove(index)}
                  className="ml-1 hover:bg-muted rounded-sm p-0.5"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </Badge>
          ))}
        </div>
      )}

      {/* Add new expression */}
      {!disabled && (
        <div className="flex flex-wrap items-center gap-2">
          {availableKeys.length > 0 ? (
            <Select value={newKey} onValueChange={setNewKey}>
              <SelectTrigger className="h-8 w-[120px]">
                <SelectValue placeholder="Key" />
              </SelectTrigger>
              <SelectContent>
                {availableKeys.map((key) => (
                  <SelectItem key={key} value={key}>
                    {key}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              value={newKey}
              onChange={(e) => setNewKey(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Key"
              className="h-8 w-[120px]"
            />
          )}

          <Select
            value={newOperator}
            onValueChange={(v) => setNewOperator(v as LabelMatchOperator)}
          >
            <SelectTrigger className="h-8 w-[120px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="In">In</SelectItem>
              <SelectItem value="NotIn">NotIn</SelectItem>
              <SelectItem value="Exists">Exists</SelectItem>
              <SelectItem value="DoesNotExist">DoesNotExist</SelectItem>
            </SelectContent>
          </Select>

          {operatorRequiresValues(newOperator) && (
            <Input
              value={newValues}
              onChange={(e) => setNewValues(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="value1, value2"
              className="h-8 w-[160px]"
            />
          )}

          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAdd}
            disabled={
              !newKey.trim() ||
              (operatorRequiresValues(newOperator) && !newValues.trim())
            }
            className="h-8"
          >
            <Plus className="h-3 w-3" />
          </Button>
        </div>
      )}
    </div>
  );
}

/**
 * A reusable Kubernetes label selector component that supports both
 * matchLabels (key=value pairs) and matchExpressions (set-based requirements).
 */
export function K8sLabelSelector({
  value,
  onChange,
  label,
  description,
  availableLabels,
  previewComponent,
  disabled = false,
  className,
}: Readonly<K8sLabelSelectorProps>) {
  const [showExpressions, setShowExpressions] = useState(
    () => (value.matchExpressions?.length ?? 0) > 0
  );

  const handleMatchLabelsChange = useCallback(
    (matchLabels: Record<string, string>) => {
      onChange({
        ...value,
        matchLabels: Object.keys(matchLabels).length > 0 ? matchLabels : undefined,
      });
    },
    [value, onChange]
  );

  const handleMatchExpressionsChange = useCallback(
    (matchExpressions: LabelMatchExpression[]) => {
      onChange({
        ...value,
        matchExpressions: matchExpressions.length > 0 ? matchExpressions : undefined,
      });
    },
    [value, onChange]
  );

  const hasSelector =
    (value.matchLabels && Object.keys(value.matchLabels).length > 0) ||
    (value.matchExpressions && value.matchExpressions.length > 0);

  return (
    <div className={cn("space-y-3", className)}>
      {label && <Label className="text-sm font-medium">{label}</Label>}
      {description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}

      <div className="space-y-4 rounded-md border p-3 bg-muted/20">
        {/* Match Labels */}
        <MatchLabelEditor
          matchLabels={value.matchLabels || {}}
          onChange={handleMatchLabelsChange}
          availableLabels={availableLabels}
          disabled={disabled}
        />

        {/* Toggle for expressions */}
        <button
          type="button"
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
          onClick={() => setShowExpressions(!showExpressions)}
          disabled={disabled}
        >
          {showExpressions ? (
            <ChevronUp className="h-3 w-3" />
          ) : (
            <ChevronDown className="h-3 w-3" />
          )}
          {showExpressions ? "Hide" : "Show"} advanced expressions
        </button>

        {/* Match Expressions */}
        {showExpressions && (
          <MatchExpressionEditor
            expressions={value.matchExpressions || []}
            onChange={handleMatchExpressionsChange}
            availableLabels={availableLabels}
            disabled={disabled}
          />
        )}
      </div>

      {/* Preview */}
      {previewComponent && hasSelector && (
        <div className="rounded-md border border-dashed p-2 bg-muted/10">
          {previewComponent}
        </div>
      )}

      {/* Empty state */}
      {!hasSelector && !disabled && (
        <p className="text-xs text-muted-foreground italic">
          No selectors configured. Add labels or expressions to filter resources.
        </p>
      )}
    </div>
  );
}

/**
 * Utility function to check if a resource matches a label selector.
 * Implements Kubernetes-style label selector matching.
 */
export function matchesLabelSelector(
  resourceLabels: Record<string, string> | undefined,
  selector: LabelSelectorValue
): boolean {
  const labels = resourceLabels || {};

  // Check matchLabels (AND logic - all must match exactly)
  if (selector.matchLabels) {
    for (const [key, expectedValue] of Object.entries(selector.matchLabels)) {
      if (labels[key] !== expectedValue) {
        return false;
      }
    }
  }

  // Check matchExpressions (AND logic with operators)
  if (selector.matchExpressions) {
    for (const expr of selector.matchExpressions) {
      const actualValue = labels[expr.key];
      const hasKey = expr.key in labels;

      switch (expr.operator) {
        case "In":
          if (!hasKey || !expr.values?.includes(actualValue)) {
            return false;
          }
          break;
        case "NotIn":
          if (hasKey && expr.values?.includes(actualValue)) {
            return false;
          }
          break;
        case "Exists":
          if (!hasKey) {
            return false;
          }
          break;
        case "DoesNotExist":
          if (hasKey) {
            return false;
          }
          break;
      }
    }
  }

  return true;
}
