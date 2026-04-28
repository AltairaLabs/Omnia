"use client";

/**
 * Multi-select for eval group routing (#988).
 *
 * Renders a checkbox per known group plus a free-text input for custom
 * groups. No new UI deps — composes existing Checkbox / Input / Badge
 * primitives, matching the dashboard's existing arena/import-provider
 * pattern (Set-backed selection, click row to toggle).
 *
 * Empty `value` means "use the path's built-in default" — the
 * operator-side EvalPathConfig contract. We never coerce to a default
 * inline; the parent component decides whether to send `groups: []` or
 * omit the field.
 */

import { useState, useCallback, useMemo } from "react";
import { Plus, X } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

interface GroupSelectorProps {
  /** Currently-selected group names. */
  value: string[];
  /** Group names to offer as built-in or pack-discovered options. */
  options: string[];
  /** Fires with the new selection set whenever the user toggles a
   *  checkbox or adds a custom group. */
  onChange: (next: string[]) => void;
  /** Disables all interactions while a save is in flight. */
  disabled?: boolean;
  /** ID prefix used to keep label/checkbox associations unique when
   *  two GroupSelectors render in the same form. */
  idPrefix: string;
  /** Description shown below the field — explains what an empty
   *  selection means for the path. */
  emptyHint: string;
  /** Sentinel hint shown above the option list. */
  label: string;
}

const GROUP_NAME_RE = /^[A-Za-z0-9]([A-Za-z0-9_.-]{0,61}[A-Za-z0-9])?$/;

export function GroupSelector({
  value,
  options,
  onChange,
  disabled = false,
  idPrefix,
  emptyHint,
  label,
}: Readonly<GroupSelectorProps>) {
  const [customDraft, setCustomDraft] = useState("");
  const [customError, setCustomError] = useState<string | null>(null);

  // The selection set decides which checkboxes are on. We also need
  // to know which selected entries aren't in `options` so they render
  // as removable chips above the option list (custom groups added in
  // a previous session, or set via kubectl edit).
  const selected = useMemo(() => new Set(value), [value]);
  const customSelected = useMemo(
    () => value.filter((g) => !options.includes(g)),
    [value, options],
  );

  const toggle = useCallback(
    (group: string) => {
      if (disabled) return;
      const next = selected.has(group)
        ? value.filter((g) => g !== group)
        : [...value, group];
      onChange(next);
    },
    [disabled, selected, value, onChange],
  );

  const removeCustom = useCallback(
    (group: string) => {
      if (disabled) return;
      onChange(value.filter((g) => g !== group));
    },
    [disabled, value, onChange],
  );

  const addCustom = useCallback(() => {
    const trimmed = customDraft.trim();
    if (!trimmed) return;
    if (!GROUP_NAME_RE.test(trimmed)) {
      setCustomError(
        "Group name must be alphanumeric (with _, -, .) and 1–63 characters.",
      );
      return;
    }
    if (selected.has(trimmed)) {
      setCustomError("Already selected.");
      return;
    }
    setCustomError(null);
    setCustomDraft("");
    onChange([...value, trimmed]);
  }, [customDraft, selected, value, onChange]);

  return (
    <div className="space-y-2">
      <Label className="text-sm font-medium">{label}</Label>

      {customSelected.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {customSelected.map((g) => (
            <Badge key={g} variant="secondary" className="gap-1">
              {g}
              <button
                type="button"
                aria-label={`Remove ${g}`}
                disabled={disabled}
                className="ml-0.5 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1"
                onClick={() => removeCustom(g)}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      <div className="space-y-1.5 rounded-md border p-3">
        {options.map((g) => {
          const id = `${idPrefix}-${g}`;
          return (
            <label
              key={g}
              htmlFor={id}
              className="flex items-center gap-2 cursor-pointer"
            >
              <Checkbox
                id={id}
                checked={selected.has(g)}
                disabled={disabled}
                onCheckedChange={() => toggle(g)}
              />
              <span className="text-sm">{g}</span>
            </label>
          );
        })}
      </div>

      <div className="flex items-start gap-2">
        <div className="flex-1">
          <Input
            placeholder="Add custom group…"
            value={customDraft}
            disabled={disabled}
            onChange={(e) => {
              setCustomDraft(e.target.value);
              if (customError) setCustomError(null);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                addCustom();
              }
            }}
            aria-label={`Add a custom group to ${label}`}
          />
          {customError && (
            <p className="mt-1 text-xs text-destructive">{customError}</p>
          )}
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || customDraft.trim().length === 0}
          onClick={addCustom}
        >
          <Plus className="h-4 w-4" />
          Add
        </Button>
      </div>

      <p className="text-xs text-muted-foreground">{emptyHint}</p>
    </div>
  );
}
