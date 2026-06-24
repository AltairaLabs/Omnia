"use client";

import { Key, Trash2, AlertCircle, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNow } from "date-fns";

export interface ApiKeyInfo {
  id: string;
  name: string;
  keyPrefix: string;
  role: string;
  expiresAt: string | null;
  createdAt: string;
  lastUsedAt: string | null;
  isExpired: boolean;
}

/** Renders the expiration status for an API key */
export function KeyExpiration({
  isExpired,
  expiresAt,
}: Readonly<{ isExpired: boolean; expiresAt: string | null }>) {
  if (isExpired) {
    return <Badge variant="destructive">Expired</Badge>;
  }
  if (expiresAt) {
    return (
      <span className="text-sm text-muted-foreground flex items-center gap-1">
        <Clock className="h-3 w-3" />
        {formatDistanceToNow(new Date(expiresAt), { addSuffix: true })}
      </span>
    );
  }
  return <span className="text-sm text-muted-foreground">Never</span>;
}

/** Renders the API keys content based on loading/error/data state */
export function ApiKeysContent({
  isLoading,
  error,
  keys,
  canCreateDelete,
  isFileMode,
  onDeleteKey,
}: Readonly<{
  isLoading: boolean;
  error: Error | null;
  keys: ApiKeyInfo[] | undefined;
  canCreateDelete: boolean;
  isFileMode: boolean;
  onDeleteKey: (id: string) => void;
}>) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center gap-2 text-destructive">
        <AlertCircle className="h-4 w-4" />
        <span>Failed to load API keys</span>
      </div>
    );
  }

  if (!keys || keys.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <Key className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p>No API keys yet</p>
        {canCreateDelete && (
          <p className="text-sm mt-1">
            Create one to access the API programmatically
          </p>
        )}
        {isFileMode && (
          <p className="text-sm mt-1">
            Contact your administrator to provision API keys
          </p>
        )}
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Key</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>Created</TableHead>
          <TableHead>Last Used</TableHead>
          <TableHead>Expires</TableHead>
          {canCreateDelete && <TableHead className="w-[50px]" />}
        </TableRow>
      </TableHeader>
      <TableBody>
        {keys.map((key) => (
          <TableRow key={key.id}>
            <TableCell className="font-medium">{key.name}</TableCell>
            <TableCell>
              <code className="text-xs bg-muted px-1 py-0.5 rounded">
                {key.keyPrefix}
              </code>
            </TableCell>
            <TableCell>
              <Badge variant="outline">{key.role}</Badge>
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatDistanceToNow(new Date(key.createdAt), {
                addSuffix: true,
              })}
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {key.lastUsedAt
                ? formatDistanceToNow(new Date(key.lastUsedAt), {
                    addSuffix: true,
                  })
                : "Never"}
            </TableCell>
            <TableCell>
              <KeyExpiration isExpired={key.isExpired} expiresAt={key.expiresAt} />
            </TableCell>
            {canCreateDelete && (
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-destructive hover:text-destructive"
                  onClick={() => onDeleteKey(key.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </TableCell>
            )}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
