"use client";

import { useCallback, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { TopologyGraph, NotesPanel } from "@/components/topology";
import { NamespaceFilter } from "@/components/filters";
import { Skeleton } from "@/components/ui/skeleton";
import { Bot, FileText, Package, Wrench } from "lucide-react";
import { useAgents, usePromptPacks, useToolRegistries } from "@/hooks";
import { loadNotes, setNote, deleteNote, type NotesMap } from "@/lib/notes-storage";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

export default function TopologyPage() {
  const router = useRouter();
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);

  // Notes state - initialize from localStorage
  const [notes, setNotes] = useState<NotesMap>(() => {
    if (typeof window === "undefined") return {};
    return loadNotes();
  });
  const [noteDialogOpen, setNoteDialogOpen] = useState(false);
  const [editingNote, setEditingNote] = useState<{
    type: string;
    namespace: string;
    name: string;
  } | null>(null);
  const [noteText, setNoteText] = useState("");

  const { data: agents, isLoading: agentsLoading } = useAgents();
  const { data: promptPacks, isLoading: promptPacksLoading } = usePromptPacks();
  const { data: toolRegistries, isLoading: toolRegistriesLoading } = useToolRegistries();

  const isLoading = agentsLoading || promptPacksLoading || toolRegistriesLoading;

  // Extract unique namespaces from all resources
  const allNamespaces = useMemo(() => {
    const namespaces = new Set<string>();
    agents?.forEach((a) => {
      if (a.metadata.namespace) namespaces.add(a.metadata.namespace);
    });
    promptPacks?.forEach((p) => {
      if (p.metadata.namespace) namespaces.add(p.metadata.namespace);
    });
    toolRegistries?.forEach((t) => {
      if (t.metadata.namespace) namespaces.add(t.metadata.namespace);
    });
    return [...namespaces].sort();
  }, [agents, promptPacks, toolRegistries]);

  const handleNamespaceChange = useCallback((namespaces: string[]) => {
    setSelectedNamespaces(namespaces);
  }, []);

  // Note handlers
  const handleNoteEdit = useCallback((type: string, namespace: string, name: string) => {
    const key = `${type}/${namespace}/${name}`;
    const existingNote = notes[key]?.note || "";
    setEditingNote({ type, namespace, name });
    setNoteText(existingNote);
    setNoteDialogOpen(true);
  }, [notes]);

  const handleNoteDelete = useCallback((type: string, namespace: string, name: string) => {
    deleteNote(type, namespace, name);
    setNotes(loadNotes());
  }, []);

  const handleNoteSave = useCallback(() => {
    if (editingNote && noteText.trim()) {
      setNote(editingNote.type, editingNote.namespace, editingNote.name, noteText.trim());
      setNotes(loadNotes());
    }
    setNoteDialogOpen(false);
    setEditingNote(null);
    setNoteText("");
  }, [editingNote, noteText]);

  const handleNoteDialogClose = useCallback(() => {
    setNoteDialogOpen(false);
    setEditingNote(null);
    setNoteText("");
  }, []);

  // Filter resources by namespace
  const filteredAgents = useMemo(() => {
    if (!agents || selectedNamespaces.length === 0) return agents || [];
    return agents.filter((a) => a.metadata.namespace && selectedNamespaces.includes(a.metadata.namespace));
  }, [agents, selectedNamespaces]);

  const filteredPromptPacks = useMemo(() => {
    if (!promptPacks || selectedNamespaces.length === 0) return promptPacks || [];
    return promptPacks.filter((p) => p.metadata.namespace && selectedNamespaces.includes(p.metadata.namespace));
  }, [promptPacks, selectedNamespaces]);

  const filteredToolRegistries = useMemo(() => {
    if (!toolRegistries || selectedNamespaces.length === 0) return toolRegistries || [];
    return toolRegistries.filter((t) => t.metadata.namespace && selectedNamespaces.includes(t.metadata.namespace));
  }, [toolRegistries, selectedNamespaces]);

  const handleNodeClick = useCallback(
    (type: string, name: string, namespace: string) => {
      switch (type) {
        case "agent":
          router.push(`/agents/${name}?namespace=${namespace}`);
          break;
        case "promptpack":
          router.push(`/promptpacks/${name}?namespace=${namespace}`);
          break;
        case "tools":
          router.push(`/tools/${name}?namespace=${namespace}`);
          break;
      }
    },
    [router]
  );

  // Calculate stats from filtered data
  const totalTools = filteredToolRegistries.reduce(
    (sum, r) => sum + (r.status?.discoveredToolsCount || 0),
    0
  );

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Topology"
        description="Visualize relationships between Agents, PromptPacks, and Tools"
      />

      <div className="flex-1 p-6 space-y-4">
        {/* Legend and Filter */}
        <div className="flex flex-wrap items-center justify-between gap-y-2">
          <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
            {/* Resource types */}
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded bg-blue-500" />
              <Bot className="h-4 w-4 text-blue-600" />
              <span className="text-muted-foreground">Agents ({filteredAgents.length})</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded bg-purple-500" />
              <FileText className="h-4 w-4 text-purple-600" />
              <span className="text-muted-foreground">PromptPacks ({filteredPromptPacks.length})</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded bg-orange-500" />
              <Package className="h-4 w-4 text-orange-600" />
              <span className="text-muted-foreground">ToolRegistries ({filteredToolRegistries.length})</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded bg-teal-500" />
              <Wrench className="h-4 w-4 text-teal-600" />
              <span className="text-muted-foreground">Tools ({totalTools})</span>
            </div>

            {/* Status indicators */}
            <div className="border-l pl-6 flex items-center gap-4">
              <span className="text-muted-foreground font-medium">Status:</span>
              <div className="flex items-center gap-1.5">
                <div className="w-2.5 h-2.5 rounded-full bg-green-500" />
                <span className="text-muted-foreground">Healthy</span>
              </div>
              <div className="flex items-center gap-1.5">
                <div className="w-2.5 h-2.5 rounded-full bg-yellow-500" />
                <span className="text-muted-foreground">Pending</span>
              </div>
              <div className="flex items-center gap-1.5">
                <div className="w-2.5 h-2.5 rounded-full bg-red-500" />
                <span className="text-muted-foreground">Failed</span>
              </div>
            </div>
          </div>

          {/* Namespace Filter and Notes */}
          <div className="flex items-center gap-2">
            <NamespaceFilter
              namespaces={allNamespaces}
              selectedNamespaces={selectedNamespaces}
              onSelectionChange={handleNamespaceChange}
            />
            <NotesPanel
              resources={[
                ...filteredAgents.map((a) => ({
                  type: "agent" as const,
                  namespace: a.metadata.namespace || "default",
                  name: a.metadata.name,
                })),
                ...filteredPromptPacks.map((p) => ({
                  type: "promptpack" as const,
                  namespace: p.metadata.namespace || "default",
                  name: p.metadata.name,
                })),
                ...filteredToolRegistries.map((t) => ({
                  type: "toolregistry" as const,
                  namespace: t.metadata.namespace || "default",
                  name: t.metadata.name,
                })),
              ]}
              selectedNamespaces={selectedNamespaces}
            />
          </div>
        </div>

        {/* Graph */}
        <div className="flex-1 min-h-[600px] border rounded-lg bg-card">
          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <Skeleton className="w-full h-full" />
            </div>
          ) : (
            <TopologyGraph
              agents={filteredAgents}
              promptPacks={filteredPromptPacks}
              toolRegistries={filteredToolRegistries}
              onNodeClick={handleNodeClick}
              notes={notes}
              onNoteEdit={handleNoteEdit}
              onNoteDelete={handleNoteDelete}
              className="w-full h-[600px]"
            />
          )}
        </div>
      </div>

      {/* Note Edit Dialog */}
      <Dialog open={noteDialogOpen} onOpenChange={(open) => !open && handleNoteDialogClose()}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>
              {notes[`${editingNote?.type}/${editingNote?.namespace}/${editingNote?.name}`]
                ? "Edit Note"
                : "Add Note"}
            </DialogTitle>
            <DialogDescription>
              {editingNote && (
                <>
                  Add a note for <span className="font-medium">{editingNote.name}</span>{" "}
                  ({editingNote.type} in {editingNote.namespace})
                </>
              )}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <Textarea
              placeholder="Enter your note here..."
              value={noteText}
              onChange={(e) => setNoteText(e.target.value)}
              rows={4}
              className="resize-none"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={handleNoteDialogClose}>
              Cancel
            </Button>
            <Button onClick={handleNoteSave} disabled={!noteText.trim()}>
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
