"use client";

import { useEffect, useMemo } from "react";
import { Filter } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";

interface NamespaceFilterProps {
  /** All available namespaces */
  namespaces: string[];
  /** Currently selected namespaces */
  selectedNamespaces: string[];
  /** Callback when selection changes */
  onSelectionChange: (namespaces: string[]) => void;
}

export function NamespaceFilter({
  namespaces,
  selectedNamespaces,
  onSelectionChange,
}: NamespaceFilterProps) {
  // Sort namespaces alphabetically
  const sortedNamespaces = useMemo(
    () => [...namespaces].sort((a, b) => a.localeCompare(b)),
    [namespaces]
  );

  // Initialize with all selected if empty
  useEffect(() => {
    if (selectedNamespaces.length === 0 && namespaces.length > 0) {
      onSelectionChange(namespaces);
    }
  }, [namespaces, selectedNamespaces.length, onSelectionChange]);

  const allSelected = selectedNamespaces.length === namespaces.length;
  const noneSelected = selectedNamespaces.length === 0;

  const handleToggle = (namespace: string) => {
    if (selectedNamespaces.includes(namespace)) {
      onSelectionChange(selectedNamespaces.filter((ns) => ns !== namespace));
    } else {
      onSelectionChange([...selectedNamespaces, namespace]);
    }
  };

  const handleSelectAll = () => {
    onSelectionChange(namespaces);
  };

  const handleSelectNone = () => {
    onSelectionChange([]);
  };

  // Determine button label
  const getButtonLabel = (): string => {
    if (allSelected) return "All namespaces";
    if (noneSelected) return "No namespaces";
    if (selectedNamespaces.length === 1) return selectedNamespaces[0];
    return `${selectedNamespaces.length} namespaces`;
  };
  const buttonLabel = getButtonLabel();

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8 gap-2">
          <Filter className="h-3.5 w-3.5" />
          <span className="text-xs">{buttonLabel}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-3" align="start">
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Namespaces</span>
            <div className="flex gap-2">
              <button
                onClick={handleSelectAll}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                All
              </button>
              <span className="text-muted-foreground">|</span>
              <button
                onClick={handleSelectNone}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                None
              </button>
            </div>
          </div>
          <div className="space-y-2">
            {sortedNamespaces.map((namespace) => (
              <div key={namespace} className="flex items-center space-x-2">
                <Checkbox
                  id={`ns-${namespace}`}
                  checked={selectedNamespaces.includes(namespace)}
                  onCheckedChange={() => handleToggle(namespace)}
                />
                <Label
                  htmlFor={`ns-${namespace}`}
                  className="text-sm font-normal cursor-pointer"
                >
                  {namespace}
                </Label>
              </div>
            ))}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
