"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Loader2, Server } from "lucide-react";
import { useProviders } from "@/hooks/use-providers";
import { insertBindingAnnotations } from "@/lib/arena/provider-binding";
import type { Provider } from "@/types";

export interface BindProviderDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** Path of the provider file to bind */
  readonly filePath: string;
  /** Get file content (from store or API) */
  readonly getContent: () => Promise<string>;
  /** Save updated file content */
  readonly saveContent: (content: string) => Promise<void>;
  /** Called after successful binding to refresh tree */
  readonly onBound: () => void;
}

/**
 * Dialog for binding an unbound provider YAML file to a cluster Provider.
 * Shows a single-select list of available providers and inserts binding annotations.
 */
export function BindProviderDialog({
  open,
  onOpenChange,
  filePath,
  getContent,
  saveContent,
  onBound,
}: BindProviderDialogProps) {
  const { data: providers, isLoading: loadingProviders } = useProviders();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [binding, setBinding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleOpenChange = (newOpen: boolean) => {
    if (newOpen) {
      setSelectedId(null);
      setError(null);
    }
    onOpenChange(newOpen);
  };

  const getProviderId = (provider: Provider) =>
    `${provider.metadata.namespace || "default"}/${provider.metadata.name}`;

  const handleBind = async () => {
    if (!selectedId || !providers) return;

    const provider = providers.find((p) => getProviderId(p) === selectedId);
    if (!provider) return;

    setBinding(true);
    setError(null);

    try {
      const content = await getContent();
      const updated = insertBindingAnnotations(
        content,
        provider.metadata.name,
        provider.metadata.namespace || "default"
      );
      await saveContent(updated);
      onBound();
      handleOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to bind provider");
    } finally {
      setBinding(false);
    }
  };

  const fileName = filePath.split("/").pop() || filePath;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Bind to Provider</DialogTitle>
          <DialogDescription>
            Select a cluster provider to bind &quot;{fileName}&quot; to.
          </DialogDescription>
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
            <div className="space-y-2 max-h-[300px] overflow-y-auto">
              {providers.map((provider) => {
                const id = getProviderId(provider);
                const isSelected = selectedId === id;

                return (
                  <button
                    key={id}
                    type="button"
                    onClick={() => setSelectedId(id)}
                    className={`flex items-center space-x-3 p-2 rounded-md w-full text-left cursor-pointer transition-colors ${
                      isSelected
                        ? "bg-primary/10 ring-1 ring-primary"
                        : "hover:bg-muted/50"
                    }`}
                  >
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
                  </button>
                );
              })}
            </div>
          )}

          {error && <p className="text-sm text-destructive mt-2">{error}</p>}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={binding}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={handleBind}
            disabled={binding || !selectedId}
          >
            {binding && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Bind
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
