"use client";

import { useState, useMemo } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Badge } from "@/components/ui/badge";
import { Loader2, Wrench, ChevronLeft, ChevronRight } from "lucide-react";
import { useToolRegistries } from "@/hooks/use-tool-registries";
import {
  convertToolToArena,
  generateToolFilename,
} from "@/lib/arena/import-converters";
import type { ToolRegistry } from "@/types";

export interface ImportToolDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentPath: string | null;
  onImport: (files: { name: string; content: string }[]) => Promise<void>;
}

type WizardStep = "selectRegistry" | "selectTools";

/**
 * Two-step wizard dialog for importing tools from registries.
 * Step 1: Select a tool registry
 * Step 2: Select tools from the selected registry
 */
export function ImportToolDialog({
  open,
  onOpenChange,
  parentPath,
  onImport,
}: ImportToolDialogProps) {
  const { data: registries, isLoading: loadingRegistries } = useToolRegistries();
  const [step, setStep] = useState<WizardStep>("selectRegistry");
  const [selectedRegistryName, setSelectedRegistryName] = useState<string | null>(null);
  const [selectedToolNames, setSelectedToolNames] = useState<Set<string>>(new Set());
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Get selected registry
  const selectedRegistry = useMemo(() => {
    if (!registries || !selectedRegistryName) return null;
    return registries.find((r) => r.metadata.name === selectedRegistryName) || null;
  }, [registries, selectedRegistryName]);

  // Get discovered tools from selected registry
  const discoveredTools = useMemo(() => {
    if (!selectedRegistry?.status?.discoveredTools) return [];
    return selectedRegistry.status.discoveredTools;
  }, [selectedRegistry]);

  // Reset state when dialog opens/closes
  const handleOpenChange = (newOpen: boolean) => {
    if (newOpen) {
      setStep("selectRegistry");
      setSelectedRegistryName(null);
      setSelectedToolNames(new Set());
      setError(null);
    }
    onOpenChange(newOpen);
  };

  // Toggle tool selection
  const toggleTool = (toolName: string) => {
    setSelectedToolNames((prev) => {
      const next = new Set(prev);
      if (next.has(toolName)) {
        next.delete(toolName);
      } else {
        next.add(toolName);
      }
      return next;
    });
  };

  // Select/deselect all tools
  const toggleAllTools = () => {
    if (selectedToolNames.size === discoveredTools.length) {
      setSelectedToolNames(new Set());
    } else {
      setSelectedToolNames(new Set(discoveredTools.map((t) => t.name)));
    }
  };

  // Get count of tools in a registry
  const getToolCount = (registry: ToolRegistry) => {
    return registry.status?.discoveredToolsCount || 0;
  };

  // Handle next step
  const handleNext = () => {
    if (step === "selectRegistry" && selectedRegistryName) {
      setSelectedToolNames(new Set());
      setStep("selectTools");
    }
  };

  // Handle back
  const handleBack = () => {
    if (step === "selectTools") {
      setStep("selectRegistry");
    }
  };

  // Handle import
  const handleImport = async () => {
    if (!selectedRegistry || selectedToolNames.size === 0) return;

    setImporting(true);
    setError(null);

    try {
      const selectedTools = discoveredTools.filter((t) =>
        selectedToolNames.has(t.name)
      );

      const registryName = selectedRegistry.metadata.name;
      const registryNamespace = selectedRegistry.metadata.namespace || "default";

      const files = selectedTools.map((tool) => ({
        name: generateToolFilename(tool, registryName),
        content: convertToolToArena(tool, { registryName, registryNamespace }),
      }));

      await onImport(files);
      handleOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to import tools");
    } finally {
      setImporting(false);
    }
  };

  const baseDescription = parentPath
    ? `Import tools into "${parentPath}"`
    : "Import tools at the root level";

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>
            Import Tools ({step === "selectRegistry" ? "1/2" : "2/2"})
          </DialogTitle>
          <DialogDescription>{baseDescription}</DialogDescription>
        </DialogHeader>

        <div className="py-4">
          {loadingRegistries ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              <span className="ml-2 text-sm text-muted-foreground">
                Loading tool registries...
              </span>
            </div>
          ) : step === "selectRegistry" ? (
            // Step 1: Select Registry
            <div className="space-y-4">
              {!registries || registries.length === 0 ? (
                <div className="text-sm text-muted-foreground py-4 text-center">
                  No tool registries found in the workspace.
                </div>
              ) : (
                <>
                  <div className="text-sm text-muted-foreground">
                    Select a tool registry:
                  </div>

                  <RadioGroup
                    value={selectedRegistryName || ""}
                    onValueChange={setSelectedRegistryName}
                    className="max-h-[300px] overflow-y-auto"
                  >
                    {registries.map((registry) => {
                      const toolCount = getToolCount(registry);
                      const name = registry.metadata.name;

                      return (
                        <label
                          key={name}
                          htmlFor={name}
                          className="flex items-center space-x-3 p-3 rounded-md hover:bg-muted/50 cursor-pointer"
                        >
                          <RadioGroupItem value={name} id={name} />
                          <div className="flex-1 min-w-0">
                            <span className="text-sm font-medium">
                              {name}
                            </span>
                          </div>
                          <span className="text-xs text-muted-foreground">
                            {toolCount} {toolCount === 1 ? "tool" : "tools"}
                          </span>
                        </label>
                      );
                    })}
                  </RadioGroup>
                </>
              )}
            </div>
          ) : (
            // Step 2: Select Tools
            <div className="space-y-4">
              <div className="text-sm text-muted-foreground">
                Select tools from &quot;{selectedRegistryName}&quot;:
              </div>

              {discoveredTools.length === 0 ? (
                <div className="text-sm text-muted-foreground py-4 text-center">
                  No tools discovered in this registry.
                </div>
              ) : (
                <>
                  {/* Select all checkbox */}
                  <div className="flex items-center space-x-2 pb-2 border-b">
                    <Checkbox
                      id="select-all-tools"
                      checked={selectedToolNames.size === discoveredTools.length}
                      onCheckedChange={toggleAllTools}
                    />
                    <Label
                      htmlFor="select-all-tools"
                      className="text-sm font-medium cursor-pointer"
                    >
                      Select all
                    </Label>
                  </div>

                  {/* Tool list */}
                  <div className="max-h-[300px] overflow-y-auto space-y-2">
                    {discoveredTools.map((tool) => {
                      const isSelected = selectedToolNames.has(tool.name);
                      const isAvailable = tool.status === "Available";

                      return (
                        <label
                          key={tool.name}
                          htmlFor={tool.name}
                          className="flex items-center space-x-3 p-2 rounded-md hover:bg-muted/50 cursor-pointer"
                        >
                          <Checkbox
                            id={tool.name}
                            checked={isSelected}
                            onCheckedChange={() => toggleTool(tool.name)}
                          />
                          <Wrench className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                          <div className="flex-1 min-w-0">
                            <div className="text-sm font-medium truncate">
                              {tool.name}
                            </div>
                            {tool.description && (
                              <div className="text-xs text-muted-foreground truncate">
                                {tool.description}
                              </div>
                            )}
                          </div>
                          <Badge
                            variant={isAvailable ? "default" : "secondary"}
                            className={
                              isAvailable
                                ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
                                : "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400"
                            }
                          >
                            {isAvailable ? "Available" : "Unavailable"}
                          </Badge>
                        </label>
                      );
                    })}
                  </div>

                  {/* Selection count */}
                  <div className="text-sm text-muted-foreground pt-2 border-t">
                    {selectedToolNames.size} of {discoveredTools.length} selected
                  </div>
                </>
              )}
            </div>
          )}

          {error && <p className="text-sm text-destructive mt-2">{error}</p>}
        </div>

        <DialogFooter className="flex justify-between sm:justify-between">
          {step === "selectTools" ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={handleBack}
                disabled={importing}
              >
                <ChevronLeft className="mr-1 h-4 w-4" />
                Back
              </Button>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => handleOpenChange(false)}
                  disabled={importing}
                >
                  Cancel
                </Button>
                <Button
                  type="button"
                  onClick={handleImport}
                  disabled={importing || selectedToolNames.size === 0}
                >
                  {importing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Import
                </Button>
              </div>
            </>
          ) : (
            <>
              <div />
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => handleOpenChange(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="button"
                  onClick={handleNext}
                  disabled={!selectedRegistryName}
                >
                  Next
                  <ChevronRight className="ml-1 h-4 w-4" />
                </Button>
              </div>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
