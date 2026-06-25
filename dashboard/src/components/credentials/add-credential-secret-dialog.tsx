"use client";

import { useState, useCallback } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useCreateSecret, useNamespaces } from "@/hooks/resources";

// Provider templates for common API key configurations
const PROVIDER_TEMPLATES: Record<string, { key: string; label: string }> = {
  claude: { key: "ANTHROPIC_API_KEY", label: "Anthropic (Claude)" },
  openai: { key: "OPENAI_API_KEY", label: "OpenAI" },
  gemini: { key: "GEMINI_API_KEY", label: "Google (Gemini)" },
  custom: { key: "", label: "Custom" },
};

interface KeyValuePair {
  id: string;
  key: string;
  value: string;
}

let pairIdCounter = 0;
function createPair(key = "", value = ""): KeyValuePair {
  return { id: `add-pair-${++pairIdCounter}`, key, value };
}

export interface AddCredentialSecretDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  namespace?: string;
  onCreated: (name: string) => void;
}

export function AddCredentialSecretDialog({
  open,
  onOpenChange,
  namespace: initialNamespace,
  onCreated,
}: Readonly<AddCredentialSecretDialogProps>) {
  const { data: namespaces } = useNamespaces();
  const createMutation = useCreateSecret();

  const [namespace, setNamespace] = useState(initialNamespace ?? "default");
  const [secretName, setSecretName] = useState("");
  const [providerType, setProviderType] = useState("custom");
  const [keyValuePairs, setKeyValuePairs] = useState<KeyValuePair[]>([createPair()]);

  const resetForm = useCallback(() => {
    setNamespace(initialNamespace ?? "default");
    setSecretName("");
    setProviderType("custom");
    setKeyValuePairs([createPair()]);
  }, [initialNamespace]);

  const handleProviderChange = useCallback((type: string) => {
    setProviderType(type);
    if (type !== "custom") {
      const template = PROVIDER_TEMPLATES[type];
      setKeyValuePairs([createPair(template.key, "")]);
    }
  }, []);

  const addKeyValuePair = useCallback(() => {
    setKeyValuePairs((prev) => [...prev, createPair()]);
  }, []);

  const removeKeyValuePair = useCallback((index: number) => {
    setKeyValuePairs((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const updateKeyValuePair = useCallback(
    (index: number, field: "key" | "value", value: string) => {
      setKeyValuePairs((prev) =>
        prev.map((pair, i) => (i === index ? { ...pair, [field]: value } : pair))
      );
    },
    []
  );

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        resetForm();
      }
      onOpenChange(nextOpen);
    },
    [onOpenChange, resetForm]
  );

  const handleSubmit = useCallback(async () => {
    const data: Record<string, string> = {};
    for (const pair of keyValuePairs) {
      if (pair.key && pair.value) {
        data[pair.key] = pair.value;
      }
    }

    if (Object.keys(data).length === 0) {
      return;
    }

    try {
      await createMutation.mutateAsync({
        namespace,
        name: secretName,
        data,
        providerType: providerType === "custom" ? undefined : providerType,
      });

      onCreated(secretName);
      resetForm();
    } catch {
      // Error is handled by mutation state
    }
  }, [namespace, secretName, keyValuePairs, providerType, createMutation, onCreated, resetForm]);

  const isFormValid =
    namespace &&
    secretName &&
    keyValuePairs.some((pair) => pair.key && pair.value);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Add Provider Credentials</DialogTitle>
          <DialogDescription>
            Create a new Kubernetes Secret with API credentials for an LLM
            provider.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="add-cred-namespace">Namespace</Label>
              <Select value={namespace} onValueChange={setNamespace}>
                <SelectTrigger id="add-cred-namespace">
                  <SelectValue placeholder="Select namespace" />
                </SelectTrigger>
                <SelectContent>
                  {(namespaces ?? [namespace]).map((ns) => (
                    <SelectItem key={ns} value={ns}>
                      {ns}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="add-cred-provider">Provider Template</Label>
              <Select value={providerType} onValueChange={handleProviderChange}>
                <SelectTrigger id="add-cred-provider">
                  <SelectValue placeholder="Select provider" />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(PROVIDER_TEMPLATES).map(([key, template]) => (
                    <SelectItem key={key} value={key}>
                      {template.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="add-cred-secret-name">Secret Name</Label>
            <Input
              id="add-cred-secret-name"
              placeholder="e.g., anthropic-credentials"
              value={secretName}
              onChange={(e) => setSecretName(e.target.value.toLowerCase())}
            />
            <p className="text-xs text-muted-foreground">
              Lowercase letters, numbers, and hyphens only
            </p>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Key-Value Pairs</Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={addKeyValuePair}
              >
                <Plus className="h-3 w-3 mr-1" />
                Add Key
              </Button>
            </div>

            <div className="space-y-2">
              {keyValuePairs.map((pair, index) => (
                <div key={pair.id} className="flex gap-2">
                  <Input
                    placeholder="Key (e.g., OPENAI_API_KEY)"
                    value={pair.key}
                    onChange={(e) => updateKeyValuePair(index, "key", e.target.value)}
                    className="flex-1"
                  />
                  <Input
                    type="password"
                    placeholder="Value"
                    value={pair.value}
                    onChange={(e) => updateKeyValuePair(index, "value", e.target.value)}
                    className="flex-1"
                  />
                  {keyValuePairs.length > 1 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      onClick={() => removeKeyValuePair(index)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!isFormValid || createMutation.isPending}
          >
            {createMutation.isPending ? "Creating..." : "Create"}
          </Button>
        </DialogFooter>

        {createMutation.error && (
          <p className="text-sm text-destructive mt-2">
            {createMutation.error.message}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
