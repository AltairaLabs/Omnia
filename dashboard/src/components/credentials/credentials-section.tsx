"use client";

import { useState, useCallback } from "react";
import { KeyRound, Plus, Trash2, Pencil, AlertCircle, Link2, ExternalLink } from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useSecrets, useCreateSecret, useDeleteSecret } from "@/hooks/resources";
import { usePermissions, Permission } from "@/hooks/auth";
import { formatDistanceToNow } from "date-fns";
import type { SecretSummary } from "@/lib/data/secrets-service";
import Link from "next/link";
import { AddCredentialSecretDialog } from "./add-credential-secret-dialog";

interface KeyValuePair {
  id: string;
  key: string;
  value: string;
}

let pairIdCounter = 0;
function createPair(key = "", value = ""): KeyValuePair {
  return { id: `pair-${++pairIdCounter}`, key, value };
}

/** Renders the loading state */
function LoadingSkeleton() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-10 w-full" />
    </div>
  );
}

/** Renders empty state */
function EmptyState({ canCreate }: { canCreate: boolean }) {
  return (
    <div className="text-center py-8 text-muted-foreground">
      <KeyRound className="h-12 w-12 mx-auto mb-4 opacity-50" />
      <p>No credentials configured</p>
      {canCreate && (
        <p className="text-sm mt-1">
          Create credentials to use with your LLM providers
        </p>
      )}
    </div>
  );
}

/** Renders error state */
function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex items-center gap-2 text-destructive py-4">
      <AlertCircle className="h-4 w-4" />
      <span>{message}</span>
    </div>
  );
}

/** Renders provider references for a secret */
function ProviderReferences({ refs }: { refs: SecretSummary["referencedBy"] }) {
  if (refs.length === 0) {
    return <span className="text-muted-foreground">-</span>;
  }

  return (
    <div className="flex flex-wrap gap-1">
      {refs.map((ref) => (
        <TooltipProvider key={`${ref.namespace}/${ref.name}`}>
          <Tooltip>
            <TooltipTrigger asChild>
              <Link
                href={`/providers/${ref.name}?namespace=${ref.namespace}`}
                className="inline-flex items-center gap-1 text-xs bg-muted px-2 py-0.5 rounded hover:bg-muted/80"
              >
                <Link2 className="h-3 w-3" />
                {ref.name}
              </Link>
            </TooltipTrigger>
            <TooltipContent>
              <p>Provider: {ref.name}</p>
              <p className="text-xs text-muted-foreground">
                {ref.namespace} / {ref.type}
              </p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      ))}
    </div>
  );
}

/** Renders the credentials table */
function CredentialsTable({
  secrets,
  canEdit,
  canDelete,
  onEdit,
  onDelete,
}: {
  secrets: SecretSummary[];
  canEdit: boolean;
  canDelete: boolean;
  onEdit: (secret: SecretSummary) => void;
  onDelete: (secret: SecretSummary) => void;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Namespace</TableHead>
          <TableHead>Name</TableHead>
          <TableHead>Keys</TableHead>
          <TableHead>Used By</TableHead>
          <TableHead>Modified</TableHead>
          {(canEdit || canDelete) && <TableHead className="w-[100px]" />}
        </TableRow>
      </TableHeader>
      <TableBody>
        {secrets.map((secret) => (
          <TableRow key={`${secret.namespace}/${secret.name}`}>
            <TableCell>
              <Badge variant="outline">{secret.namespace}</Badge>
            </TableCell>
            <TableCell className="font-medium">{secret.name}</TableCell>
            <TableCell>
              <div className="flex flex-wrap gap-1">
                {secret.keys.map((key) => (
                  <code
                    key={key}
                    className="text-xs bg-muted px-1.5 py-0.5 rounded"
                  >
                    {key}
                  </code>
                ))}
              </div>
            </TableCell>
            <TableCell>
              <ProviderReferences refs={secret.referencedBy} />
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {secret.modifiedAt
                ? formatDistanceToNow(new Date(secret.modifiedAt), {
                    addSuffix: true,
                  })
                : "-"}
            </TableCell>
            {(canEdit || canDelete) && (
              <TableCell>
                <div className="flex gap-1">
                  {canEdit && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      onClick={() => onEdit(secret)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                  )}
                  {canDelete && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => onDelete(secret)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </TableCell>
            )}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export function CredentialsSection() {
  const { can } = usePermissions();
  const canView = can(Permission.CREDENTIALS_VIEW);
  const canCreate = can(Permission.CREDENTIALS_CREATE);
  const canEdit = can(Permission.CREDENTIALS_EDIT);
  const canDelete = can(Permission.CREDENTIALS_DELETE);

  // Queries
  const { data: secrets, isLoading, error } = useSecrets();

  // Mutations
  const createMutation = useCreateSecret();
  const deleteMutation = useDeleteSecret();

  // Dialog state
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showEditDialog, setShowEditDialog] = useState(false);
  const [editingSecret, setEditingSecret] = useState<SecretSummary | null>(null);
  const [deleteSecret, setDeleteSecret] = useState<SecretSummary | null>(null);

  // Edit form state (create is handled by AddCredentialSecretDialog)
  const [editNamespace, setEditNamespace] = useState("default");
  const [editSecretName, setEditSecretName] = useState("");
  const [keyValuePairs, setKeyValuePairs] = useState<KeyValuePair[]>([
    createPair(),
  ]);

  // Add key-value pair
  const addKeyValuePair = useCallback(() => {
    setKeyValuePairs((prev) => [...prev, createPair()]);
  }, []);

  // Remove key-value pair
  const removeKeyValuePair = useCallback((index: number) => {
    setKeyValuePairs((prev) => prev.filter((_, i) => i !== index));
  }, []);

  // Update key-value pair
  const updateKeyValuePair = useCallback(
    (index: number, field: "key" | "value", value: string) => {
      setKeyValuePairs((prev) =>
        prev.map((pair, i) =>
          i === index ? { ...pair, [field]: value } : pair
        )
      );
    },
    []
  );

  // Open edit dialog
  const handleOpenEdit = useCallback((secret: SecretSummary) => {
    setEditingSecret(secret);
    setEditNamespace(secret.namespace);
    setEditSecretName(secret.name);
    // Set existing keys with empty values (we don't have the values)
    setKeyValuePairs(secret.keys.map((key) => createPair(key, "")));
    setShowEditDialog(true);
  }, []);

  // Handle edit submit
  const handleEditSubmit = useCallback(async () => {
    // Build data object from key-value pairs
    const data: Record<string, string> = {};
    for (const pair of keyValuePairs) {
      if (pair.key && pair.value) {
        data[pair.key] = pair.value;
      }
    }

    if (Object.keys(data).length === 0) {
      return; // No valid data
    }

    try {
      await createMutation.mutateAsync({
        namespace: editNamespace,
        name: editSecretName,
        data,
      });

      setShowEditDialog(false);
      setEditingSecret(null);
      setKeyValuePairs([createPair()]);
    } catch {
      // Error is handled by mutation state
    }
  }, [editNamespace, editSecretName, keyValuePairs, createMutation]);

  // Handle delete
  const handleDelete = useCallback(async () => {
    if (!deleteSecret) return;

    try {
      await deleteMutation.mutateAsync({
        namespace: deleteSecret.namespace,
        name: deleteSecret.name,
      });
      setDeleteSecret(null);
    } catch {
      // Error is handled by mutation state
    }
  }, [deleteSecret, deleteMutation]);

  // Check if edit form is valid
  const isEditFormValid =
    editNamespace &&
    editSecretName &&
    keyValuePairs.some((pair) => pair.key && pair.value);

  if (!canView) {
    return null;
  }

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="flex items-center gap-2">
                <KeyRound className="h-5 w-5" />
                Provider Credentials
              </CardTitle>
              <CardDescription>
                Manage API keys and credentials for LLM providers. These are
                stored as Kubernetes Secrets.
              </CardDescription>
            </div>
            {canCreate && (
              <Button onClick={() => setShowCreateDialog(true)} size="sm">
                <Plus className="h-4 w-4 mr-2" />
                Add Credentials
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {isLoading && <LoadingSkeleton />}
          {error && <ErrorState message="Failed to load credentials" />}
          {!isLoading && !error && secrets?.length === 0 && (
            <EmptyState canCreate={canCreate} />
          )}
          {!isLoading && !error && secrets && secrets.length > 0 && (
            <CredentialsTable
              secrets={secrets}
              canEdit={canEdit}
              canDelete={canDelete}
              onEdit={handleOpenEdit}
              onDelete={setDeleteSecret}
            />
          )}

          <div className="mt-4 p-3 bg-muted/50 border rounded-lg">
            <p className="text-sm text-muted-foreground">
              <strong>GitOps compatible:</strong> Secrets can also be managed
              via kubectl or GitOps. Add the label{" "}
              <code className="text-xs bg-muted px-1 py-0.5 rounded">
                omnia.altairalabs.ai/type=credentials
              </code>{" "}
              for the secret to appear here.
              <Link
                href="https://docs.omnia.altairalabs.ai/guides/credentials"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 ml-2 text-primary hover:underline"
              >
                Learn more <ExternalLink className="h-3 w-3" />
              </Link>
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Create Dialog — reuses AddCredentialSecretDialog */}
      <AddCredentialSecretDialog
        open={showCreateDialog}
        onOpenChange={setShowCreateDialog}
        onCreated={() => setShowCreateDialog(false)}
      />

      {/* Edit Dialog */}
      <Dialog open={showEditDialog} onOpenChange={setShowEditDialog}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Update Credentials</DialogTitle>
            <DialogDescription>
              Update the values for{" "}
              <code className="text-xs bg-muted px-1 py-0.5 rounded">
                {editingSecret?.namespace}/{editingSecret?.name}
              </code>
              . Leave values empty to keep them unchanged.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-4">
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
                      placeholder="Key"
                      value={pair.key}
                      onChange={(e) =>
                        updateKeyValuePair(index, "key", e.target.value)
                      }
                      className="flex-1"
                    />
                    <Input
                      type="password"
                      placeholder="New value (leave empty to keep)"
                      value={pair.value}
                      onChange={(e) =>
                        updateKeyValuePair(index, "value", e.target.value)
                      }
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
              <p className="text-xs text-muted-foreground">
                Only keys with values will be updated. Empty values are skipped.
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setShowEditDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleEditSubmit}
              disabled={!isEditFormValid || createMutation.isPending}
            >
              {createMutation.isPending ? "Updating..." : "Update"}
            </Button>
          </DialogFooter>

          {createMutation.error && (
            <p className="text-sm text-destructive mt-2">
              {createMutation.error.message}
            </p>
          )}
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={!!deleteSecret}
        onOpenChange={(open) => !open && setDeleteSecret(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Credentials?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete the secret{" "}
              <code className="text-xs bg-muted px-1 py-0.5 rounded">
                {deleteSecret?.namespace}/{deleteSecret?.name}
              </code>
              .
              {deleteSecret?.referencedBy && deleteSecret.referencedBy.length > 0 && (
                <span className="block mt-2 text-destructive">
                  Warning: This secret is used by{" "}
                  {deleteSecret.referencedBy.length} provider(s). Deleting it
                  will cause those providers to fail.
                </span>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleDelete}
            >
              {deleteMutation.isPending ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
