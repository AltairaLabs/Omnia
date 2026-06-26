"use client";

import { useMemo, useState } from "react";
import { Lock, AlertTriangle } from "lucide-react";
import { useWorkspacePermissions } from "@/hooks/use-workspace-permissions";
import { useReadOnly } from "@/hooks/use-read-only";
import { YamlBlock } from "@/components/ui/yaml-block";
import { YamlEditor } from "@/components/arena/yaml-editor";
import { Button } from "@/components/ui/button";
import { toast } from "@/hooks/use-toast";
import { ResourceUpdateError } from "@/hooks/use-tool-registry-mutations";
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
import {
  sanitizeResourceForEditor,
  buildUpdateBodyFromYaml,
  isEditableYaml,
  type EditableResource,
  type UpdateResourceBody,
} from "./editable-config-panel.utils";

const CONFLICT_MESSAGE =
  "This resource changed since you loaded it. Reload and reapply your edit.";
const FORBIDDEN_MESSAGE = "You don't have permission to edit this resource.";

export interface EditableConfigPanelProps {
  readonly kind: string;
  readonly name: string;
  readonly resource: EditableResource;
  readonly onSave: (body: UpdateResourceBody) => Promise<EditableResource>;
}

/** Map a save failure to a user-facing message. */
function messageForError(error: unknown): string {
  if (error instanceof ResourceUpdateError) {
    if (error.status === 409) return CONFLICT_MESSAGE;
    if (error.status === 403) return FORBIDDEN_MESSAGE;
    return error.message; // 400 admission / 422 validation carry the API detail
  }
  return error instanceof Error ? error.message : "Failed to save changes";
}

export function EditableConfigPanel({
  kind,
  name,
  resource,
  onSave,
}: EditableConfigPanelProps) {
  // Edit rights are workspace-scoped: a ToolRegistry (and the other CRDs this
  // panel serves) lives in a workspace, so the user's role *in that workspace*
  // governs edit — not their global role. Gating on the global usePermissions()
  // wrongly blocked workspace editors.
  const { canWrite } = useWorkspacePermissions();
  const { isReadOnly, message } = useReadOnly();

  const initialYaml = useMemo(() => sanitizeResourceForEditor(resource), [resource]);
  const [value, setValue] = useState(initialYaml);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const editable = canWrite && !isReadOnly;
  const dirty = value !== initialYaml;
  const valid = isEditableYaml(value);

  if (!editable) {
    return (
      <div className="space-y-2">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Lock className="h-3.5 w-3.5" />
          <span>{isReadOnly ? message : FORBIDDEN_MESSAGE}</span>
        </div>
        <YamlBlock data={resource} />
      </div>
    );
  }

  const handleConfirm = async () => {
    setConfirmOpen(false);
    setSaving(true);
    setError(null);
    try {
      const body = buildUpdateBodyFromYaml(value, resource.metadata?.resourceVersion);
      const saved = await onSave(body);
      setValue(sanitizeResourceForEditor(saved));
      toast({ title: "Saved", description: `${kind}/${name} updated` });
    } catch (err) {
      setError(messageForError(err));
    } finally {
      setSaving(false);
    }
  };

  const openConfirm = () => {
    if (dirty && valid) setConfirmOpen(true);
  };

  return (
    <div className="space-y-3">
      <div className="h-[600px] border rounded-md overflow-hidden">
        <YamlEditor value={value} onChange={setValue} fileType="yaml" onSave={openConfirm} />
      </div>

      {error && (
        <div className="flex items-start gap-2 text-sm text-destructive" role="alert">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <div className="flex justify-end">
        <Button disabled={!dirty || !valid || saving} onClick={openConfirm}>
          {saving ? "Saving…" : "Save"}
        </Button>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Apply changes to {kind}/{name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This updates the live resource in the cluster.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirm}>Apply</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
