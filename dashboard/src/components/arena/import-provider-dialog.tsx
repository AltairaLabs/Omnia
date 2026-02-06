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
import { Loader2, Server } from "lucide-react";
import { useProviders } from "@/hooks/use-providers";
import {
  convertProviderToArena,
  generateProviderFilename,
} from "@/lib/arena/import-converters";
import type { Provider } from "@/types";

export interface ImportProviderDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly parentPath: string | null;
  readonly onImport: (files: { name: string; content: string }[]) => Promise<void>;
}

/**
 * Dialog for importing providers from the cluster into Arena project files.
 * Shows a multi-select list of available providers.
 */
export function ImportProviderDialog({
  open,
  onOpenChange,
  parentPath,
  onImport,
}: ImportProviderDialogProps) {
  const { data: providers, isLoading: loadingProviders } = useProviders();
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset selection when dialog opens
  const handleOpenChange = (newOpen: boolean) => {
    if (newOpen) {
      setSelectedIds(new Set());
      setError(null);
    }
    onOpenChange(newOpen);
  };

  // Create a unique ID for each provider
  const getProviderId = (provider: Provider) =>
    `${provider.metadata.namespace || "default"}/${provider.metadata.name}`;

  // Toggle selection
  const toggleProvider = (provider: Provider) => {
    const id = getProviderId(provider);
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  // Select/deselect all
  const toggleAll = () => {
    if (!providers) return;
    if (selectedIds.size === providers.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(providers.map(getProviderId)));
    }
  };

  // Get selected providers
  const selectedProviders = useMemo(() => {
    if (!providers) return [];
    return providers.filter((p) => selectedIds.has(getProviderId(p)));
  }, [providers, selectedIds]);

  // Handle import
  const handleImport = async () => {
    if (selectedProviders.length === 0) return;

    setImporting(true);
    setError(null);

    try {
      const files = selectedProviders.map((provider) => ({
        name: generateProviderFilename(provider),
        content: convertProviderToArena(provider),
      }));

      await onImport(files);
      handleOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to import providers");
    } finally {
      setImporting(false);
    }
  };

  const description = parentPath
    ? `Import providers into "${parentPath}"`
    : "Import providers at the root level";

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Import Providers</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        <div className="py-4">
          {loadingProviders && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              <span className="ml-2 text-sm text-muted-foreground">
                Loading providers...
              </span>
            </div>
          )}
          {!loadingProviders && (!providers || providers.length === 0) && (
            <div className="text-sm text-muted-foreground py-4 text-center">
              No providers found in the workspace.
            </div>
          )}
          {!loadingProviders && providers && providers.length > 0 && (
            <div className="space-y-4">
              <div className="text-sm text-muted-foreground">
                Select providers to import:
              </div>

              {/* Select all checkbox */}
              <div className="flex items-center space-x-2 pb-2 border-b">
                <Checkbox
                  id="select-all"
                  checked={selectedIds.size === providers.length}
                  onCheckedChange={toggleAll}
                />
                <Label
                  htmlFor="select-all"
                  className="text-sm font-medium cursor-pointer"
                >
                  Select all
                </Label>
              </div>

              {/* Provider list */}
              <div className="max-h-[300px] overflow-y-auto space-y-2">
                {providers.map((provider) => {
                  const id = getProviderId(provider);
                  const isSelected = selectedIds.has(id);

                  return (
                    <label
                      key={id}
                      htmlFor={id}
                      className="flex items-center space-x-3 p-2 rounded-md hover:bg-muted/50 cursor-pointer"
                    >
                      <Checkbox
                        id={id}
                        checked={isSelected}
                        onCheckedChange={() => toggleProvider(provider)}
                      />
                      <Server className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium truncate">
                          {provider.metadata.name}
                        </div>
                        <div className="text-xs text-muted-foreground truncate">
                          {provider.spec.type}
                          {provider.spec.model && ` · ${provider.spec.model}`}
                        </div>
                      </div>
                    </label>
                  );
                })}
              </div>

              {/* Selection count */}
              <div className="text-sm text-muted-foreground pt-2 border-t">
                {selectedIds.size} of {providers.length} selected
              </div>
            </div>
          )}

          {error && <p className="text-sm text-destructive mt-2">{error}</p>}
        </div>

        <DialogFooter>
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
            disabled={importing || selectedIds.size === 0}
          >
            {importing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Import
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
