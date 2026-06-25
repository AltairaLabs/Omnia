"use client";

import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Plus } from "lucide-react";
import { useSecrets } from "@/hooks/resources";

export const USE_PROVIDER_DEFAULT = "__default__";

export interface SecretKeySelectProps {
  namespace?: string;
  secretName: string;
  secretKey: string;
  onSecretNameChange: (name: string) => void;
  onSecretKeyChange: (key: string) => void;
  idPrefix: string;
  onAddSecret?: () => void;
}

function EmptyState({ onAddSecret }: { onAddSecret?: () => void }) {
  return (
    <div className="text-sm text-muted-foreground space-y-2">
      <p>No credential secrets in this namespace.</p>
      {onAddSecret && (
        <Button type="button" variant="outline" size="sm" onClick={onAddSecret}>
          <Plus className="h-3 w-3 mr-1" /> Add credential secret
        </Button>
      )}
    </div>
  );
}

export function SecretKeySelect({
  namespace, secretName, secretKey, onSecretNameChange, onSecretKeyChange, idPrefix, onAddSecret,
}: Readonly<SecretKeySelectProps>) {
  const { data: secrets } = useSecrets({ namespace });
  const list = secrets ?? [];

  if (list.length === 0) {
    return <EmptyState onAddSecret={onAddSecret} />;
  }

  const selected = list.find((s) => s.name === secretName);
  const secretMissing = secretName !== "" && selected === undefined;
  const keyValue = secretKey === "" ? USE_PROVIDER_DEFAULT : secretKey;

  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="space-y-2">
        <Label htmlFor={`${idPrefix}-secret-select`}>Secret</Label>
        <Select value={secretName} onValueChange={onSecretNameChange}>
          <SelectTrigger id={`${idPrefix}-secret-select`}>
            <SelectValue placeholder="Select a secret" />
          </SelectTrigger>
          <SelectContent>
            {list.map((s) => (
              <SelectItem key={s.name} value={s.name}>{s.name}</SelectItem>
            ))}
            {secretMissing && (
              <SelectItem value={secretName}>{secretName} (not found)</SelectItem>
            )}
          </SelectContent>
        </Select>
        {onAddSecret && (
          <Button type="button" variant="link" size="sm" className="px-0 h-auto" onClick={onAddSecret}>
            <Plus className="h-3 w-3 mr-1" /> Add credential secret
          </Button>
        )}
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${idPrefix}-key-select`}>Key</Label>
        <Select
          value={keyValue}
          onValueChange={(v) => onSecretKeyChange(v === USE_PROVIDER_DEFAULT ? "" : v)}
        >
          <SelectTrigger id={`${idPrefix}-key-select`} data-testid={`${idPrefix}-key-select`}>
            <SelectValue placeholder="Use provider default" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={USE_PROVIDER_DEFAULT}>Use provider default (recommended)</SelectItem>
            {selected !== undefined && selected.keys.map((k) => (
              <SelectItem key={k} value={k}>{k}</SelectItem>
            ))}
            {secretMissing && secretKey !== "" && (
              <SelectItem value={secretKey}>{secretKey}</SelectItem>
            )}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
