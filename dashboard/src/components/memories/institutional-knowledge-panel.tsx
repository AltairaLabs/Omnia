"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { TierBadge } from "./tier-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useInstitutionalMemories,
  useCreateInstitutionalMemory,
  useDeleteInstitutionalMemory,
} from "@/hooks/use-institutional-memories";
import {
  parseJsonBulk,
  parseMarkdownBulk,
  type ParsedMemory,
} from "@/lib/memories/bulk-import-parser";
import { AlertCircle, Plus, Trash2, Upload } from "lucide-react";

export function InstitutionalKnowledgePanel() {
  const { data, isLoading, error } = useInstitutionalMemories({ limit: 500 });
  const createMemory = useCreateInstitutionalMemory();
  const deleteMemory = useDeleteInstitutionalMemory();

  const [newType, setNewType] = useState("");
  const [newContent, setNewContent] = useState("");

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newType.trim() || !newContent.trim()) return;
    await createMemory.mutateAsync({ type: newType.trim(), content: newContent.trim() });
    setNewType("");
    setNewContent("");
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Add knowledge</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleCreate} className="space-y-3" data-testid="create-form">
            <div>
              <Label htmlFor="kn-type">Type</Label>
              <Input
                id="kn-type"
                placeholder="e.g. policy, glossary, runbook"
                value={newType}
                onChange={(e) => setNewType(e.target.value)}
                required
                data-testid="create-type"
              />
            </div>
            <div>
              <Label htmlFor="kn-content">Content</Label>
              <Textarea
                id="kn-content"
                placeholder="What should agents know about this?"
                value={newContent}
                onChange={(e) => setNewContent(e.target.value)}
                required
                rows={3}
                data-testid="create-content"
              />
            </div>
            <div className="flex gap-2">
              <Button type="submit" disabled={createMemory.isPending} data-testid="create-submit">
                <Plus className="h-4 w-4 mr-2" /> Add
              </Button>
              <BulkImportDialog />
            </div>
          </form>
          {createMemory.isError && (
            <Alert variant="destructive" className="mt-3">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Failed to add</AlertTitle>
              <AlertDescription>{(createMemory.error as Error).message}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Workspace knowledge ({data?.total ?? 0})</CardTitle>
        </CardHeader>
        <CardContent>
          {renderBody({ isLoading, error, memories: data?.memories ?? [], onDelete: (id) => deleteMemory.mutate(id) })}
        </CardContent>
      </Card>
    </div>
  );
}

interface BodyProps {
  isLoading: boolean;
  error: unknown;
  memories: Array<{ id: string; type: string; content: string }>;
  onDelete: (id: string) => void;
}

function renderBody({ isLoading, error, memories, onDelete }: BodyProps) {
  if (error) {
    return (
      <Alert variant="destructive" data-testid="kn-error">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Could not load</AlertTitle>
        <AlertDescription>
          {error instanceof Error ? error.message : "Failed to load institutional memories."}
        </AlertDescription>
      </Alert>
    );
  }
  if (isLoading) {
    return <Skeleton className="w-full h-32 rounded" />;
  }
  if (memories.length === 0) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="kn-empty">
        No workspace knowledge yet. Add memories above so every agent in this workspace can see them.
      </p>
    );
  }
  return (
    <ul className="space-y-2" data-testid="kn-list">
      {memories.map((m) => (
        <li
          key={m.id}
          className="flex items-start justify-between gap-3 rounded border bg-card p-3"
        >
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <TierBadge tier="institutional" />
              <p className="text-xs font-mono text-muted-foreground">{m.type}</p>
            </div>
            <p className="text-sm whitespace-pre-wrap break-words mt-1">{m.content}</p>
          </div>
          <DeleteButton id={m.id} onConfirm={onDelete} />
        </li>
      ))}
    </ul>
  );
}

function DeleteButton({ id, onConfirm }: { id: string; onConfirm: (id: string) => void }) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Delete" data-testid={`kn-delete-${id}`}>
          <Trash2 className="h-4 w-4" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete this memory?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the entry from workspace knowledge. Agents will no longer see it.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => onConfirm(id)} data-testid={`kn-delete-confirm-${id}`}>
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function BulkImportDialog() {
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<"json" | "markdown">("json");
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<string[]>([]);
  const [summary, setSummary] = useState<string | null>(null);
  const create = useCreateInstitutionalMemory();

  const reset = () => {
    setText("");
    setErrors([]);
    setSummary(null);
  };

  const runImport = async () => {
    const parsed = tab === "json" ? parseJsonBulk(text) : parseMarkdownBulk(text);
    if (parsed.errors.length > 0) {
      setErrors(parsed.errors.map((e) => e.message));
      return;
    }
    if (parsed.memories.length === 0) {
      setErrors(["Nothing to import."]);
      return;
    }
    const results = await importAll(parsed.memories, (input) => create.mutateAsync(input));
    setSummary(`Imported ${results.ok} / ${parsed.memories.length}.`);
    setErrors(results.failures);
    if (results.ok === parsed.memories.length) {
      setTimeout(() => {
        setOpen(false);
        reset();
      }, 900);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) reset();
      }}
    >
      <DialogTrigger asChild>
        <Button variant="outline" data-testid="bulk-import-open">
          <Upload className="h-4 w-4 mr-2" /> Bulk import
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Bulk import knowledge</DialogTitle>
          <DialogDescription>
            Paste a JSON array of memories, or markdown with <code>##</code> section headers.
          </DialogDescription>
        </DialogHeader>
        <Tabs value={tab} onValueChange={(v) => setTab(v as "json" | "markdown")}>
          <TabsList>
            <TabsTrigger value="json">JSON</TabsTrigger>
            <TabsTrigger value="markdown">Markdown</TabsTrigger>
          </TabsList>
          <TabsContent value="json">
            <Textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={10}
              placeholder={`[\n  {"type":"policy","content":"API uses snake_case"}\n]`}
              data-testid="bulk-import-json"
            />
          </TabsContent>
          <TabsContent value="markdown">
            <Textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={10}
              placeholder={`## API Style\nUse snake_case.\n\n## Runbook\n...`}
              data-testid="bulk-import-markdown"
            />
          </TabsContent>
        </Tabs>
        {summary && (
          <p className="text-sm" data-testid="bulk-import-summary">
            {summary}
          </p>
        )}
        {errors.length > 0 && (
          <Alert variant="destructive" data-testid="bulk-import-errors">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Import issues</AlertTitle>
            <AlertDescription>
              <ul className="list-disc pl-5 text-sm">
                {errors.slice(0, 10).map((e) => (
                  <li key={e}>{e}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button
            onClick={runImport}
            disabled={create.isPending || !text.trim()}
            data-testid="bulk-import-submit"
          >
            Import
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

async function importAll(
  memories: ParsedMemory[],
  doCreate: (input: { type: string; content: string; confidence?: number; metadata?: Record<string, unknown>; expiresAt?: string }) => Promise<unknown>
): Promise<{ ok: number; failures: string[] }> {
  let ok = 0;
  const failures: string[] = [];
  for (const mem of memories) {
    try {
      await doCreate(mem);
      ok++;
    } catch (e) {
      failures.push(`${mem.type}: ${e instanceof Error ? e.message : String(e)}`);
    }
  }
  return { ok, failures };
}
