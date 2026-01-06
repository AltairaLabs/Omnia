"use client";

import { useState, useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Key, Plus, Trash2, Copy, Check, AlertCircle, Clock, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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
import { Skeleton } from "@/components/ui/skeleton";
import { usePermissions, Permission } from "@/hooks";
import { formatDistanceToNow } from "date-fns";

interface ApiKeyInfo {
  id: string;
  name: string;
  keyPrefix: string;
  role: string;
  expiresAt: string | null;
  createdAt: string;
  lastUsedAt: string | null;
  isExpired: boolean;
}

interface NewApiKey extends ApiKeyInfo {
  key: string;
}

interface ApiKeysResponse {
  keys: ApiKeyInfo[];
  config: {
    storeType: "memory" | "file";
    allowCreate: boolean;
    maxKeysPerUser: number;
    defaultExpirationDays: number;
  };
}

async function fetchApiKeys(): Promise<ApiKeysResponse> {
  const response = await fetch("/api/settings/api-keys");
  if (!response.ok) {
    throw new Error("Failed to fetch API keys");
  }
  return response.json();
}

async function createApiKey(data: {
  name: string;
  expiresInDays: number | null;
}): Promise<{ key: NewApiKey }> {
  const response = await fetch("/api/settings/api-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || "Failed to create API key");
  }
  return response.json();
}

async function deleteApiKey(id: string): Promise<void> {
  const response = await fetch(`/api/settings/api-keys/${id}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    throw new Error("Failed to delete API key");
  }
}

export function ApiKeysSection() {
  const { can } = usePermissions();
  const queryClient = useQueryClient();

  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showNewKeyDialog, setShowNewKeyDialog] = useState(false);
  const [newKey, setNewKey] = useState<NewApiKey | null>(null);
  const [keyCopied, setKeyCopied] = useState(false);
  const [deleteKeyId, setDeleteKeyId] = useState<string | null>(null);

  const [keyName, setKeyName] = useState("");
  const [expiration, setExpiration] = useState("90");

  const canManageKeys = can(Permission.API_KEYS_MANAGE_OWN);

  const {
    data,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["api-keys"],
    queryFn: fetchApiKeys,
    enabled: can(Permission.API_KEYS_VIEW_OWN),
  });

  const allowCreate = data?.config.allowCreate ?? true;
  const canCreateDelete = canManageKeys && allowCreate;
  const isFileMode = data?.config.storeType === "file";

  const createMutation = useMutation({
    mutationFn: createApiKey,
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setShowCreateDialog(false);
      setNewKey(response.key);
      setShowNewKeyDialog(true);
      setKeyName("");
      setExpiration("90");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteApiKey,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setDeleteKeyId(null);
    },
  });

  const handleCopyKey = useCallback(() => {
    if (newKey?.key) {
      navigator.clipboard.writeText(newKey.key);
      setKeyCopied(true);
      setTimeout(() => setKeyCopied(false), 2000);
    }
  }, [newKey]);

  const handleCreateKey = useCallback(() => {
    const expiresInDays = expiration === "never" ? null : parseInt(expiration, 10);
    createMutation.mutate({ name: keyName, expiresInDays });
  }, [keyName, expiration, createMutation]);

  if (!can(Permission.API_KEYS_VIEW_OWN)) {
    return null;
  }

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="flex items-center gap-2">
                <Key className="h-5 w-5" />
                API Keys
              </CardTitle>
              <CardDescription>
                Create and manage API keys for programmatic access to the dashboard.
              </CardDescription>
            </div>
            {canCreateDelete && (
              <Button onClick={() => setShowCreateDialog(true)} size="sm">
                <Plus className="h-4 w-4 mr-2" />
                Create Key
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {isFileMode && (
            <div className="mb-4 p-3 bg-blue-500/10 border border-blue-500/20 rounded-lg flex items-start gap-3">
              <Info className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
              <div className="text-sm">
                <p className="font-medium text-blue-600 dark:text-blue-400">
                  Keys managed via Kubernetes Secret
                </p>
                <p className="text-muted-foreground mt-1">
                  API keys are provisioned by your administrator via GitOps. Contact
                  your administrator to create or revoke keys.
                </p>
              </div>
            </div>
          )}

          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : error ? (
            <div className="flex items-center gap-2 text-destructive">
              <AlertCircle className="h-4 w-4" />
              <span>Failed to load API keys</span>
            </div>
          ) : data?.keys.length === 0 ? (
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
          ) : (
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
                {data?.keys.map((key) => (
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
                      {key.isExpired ? (
                        <Badge variant="destructive">Expired</Badge>
                      ) : key.expiresAt ? (
                        <span className="text-sm text-muted-foreground flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {formatDistanceToNow(new Date(key.expiresAt), {
                            addSuffix: true,
                          })}
                        </span>
                      ) : (
                        <span className="text-sm text-muted-foreground">Never</span>
                      )}
                    </TableCell>
                    {canCreateDelete && (
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                          onClick={() => setDeleteKeyId(key.id)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          {data && (
            <p className="text-xs text-muted-foreground mt-4">
              {data.keys.length} of {data.config.maxKeysPerUser} keys used
            </p>
          )}
        </CardContent>
      </Card>

      {/* Create Key Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create API Key</DialogTitle>
            <DialogDescription>
              Create a new API key for programmatic access. The key will only be
              shown once.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="key-name">Name</Label>
              <Input
                id="key-name"
                placeholder="My Integration"
                value={keyName}
                onChange={(e) => setKeyName(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                A descriptive name to identify this key
              </p>
            </div>

            <div className="space-y-2">
              <Label>Expiration</Label>
              <Select value={expiration} onValueChange={setExpiration}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="30">30 days</SelectItem>
                  <SelectItem value="60">60 days</SelectItem>
                  <SelectItem value="90">90 days</SelectItem>
                  <SelectItem value="180">180 days</SelectItem>
                  <SelectItem value="365">1 year</SelectItem>
                  <SelectItem value="never">Never</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowCreateDialog(false)}
            >
              Cancel
            </Button>
            <Button
              onClick={handleCreateKey}
              disabled={!keyName.trim() || createMutation.isPending}
            >
              {createMutation.isPending ? "Creating..." : "Create Key"}
            </Button>
          </DialogFooter>

          {createMutation.error && (
            <p className="text-sm text-destructive mt-2">
              {createMutation.error.message}
            </p>
          )}
        </DialogContent>
      </Dialog>

      {/* New Key Display Dialog */}
      <Dialog
        open={showNewKeyDialog}
        onOpenChange={(open) => {
          if (!open) {
            setShowNewKeyDialog(false);
            setNewKey(null);
            setKeyCopied(false);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Check className="h-5 w-5 text-green-500" />
              API Key Created
            </DialogTitle>
            <DialogDescription>
              Copy your API key now. You won&apos;t be able to see it again.
            </DialogDescription>
          </DialogHeader>

          <div className="py-4">
            <div className="flex items-center gap-2">
              <code className="flex-1 p-3 bg-muted rounded text-sm font-mono break-all">
                {newKey?.key}
              </code>
              <Button
                variant="outline"
                size="icon"
                onClick={handleCopyKey}
                className="shrink-0"
              >
                {keyCopied ? (
                  <Check className="h-4 w-4 text-green-500" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </Button>
            </div>

            <div className="mt-4 p-3 bg-yellow-500/10 border border-yellow-500/20 rounded text-sm">
              <p className="font-medium text-yellow-600 dark:text-yellow-400">
                Important
              </p>
              <p className="text-muted-foreground mt-1">
                Store this key securely. It provides access to the API with your
                permissions and cannot be retrieved after closing this dialog.
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button onClick={() => setShowNewKeyDialog(false)}>
              {keyCopied ? "Done" : "I've copied the key"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={!!deleteKeyId}
        onOpenChange={(open) => !open && setDeleteKeyId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke API Key?</AlertDialogTitle>
            <AlertDialogDescription>
              This will immediately revoke the API key. Any applications using
              this key will no longer be able to authenticate.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteKeyId && deleteMutation.mutate(deleteKeyId)}
            >
              {deleteMutation.isPending ? "Revoking..." : "Revoke Key"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
