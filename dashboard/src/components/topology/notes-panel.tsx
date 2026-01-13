"use client";

import { useState, useSyncExternalStore } from "react";
import { StickyNote, Plus, Trash2, Bot, FileText, Package } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  setNote,
  deleteNote,
  type NotesMap,
} from "@/lib/notes-storage";

interface Resource {
  type: "agent" | "promptpack" | "toolregistry";
  namespace: string;
  name: string;
}

interface NotesPanelProps {
  resources: Resource[];
  selectedNamespaces: string[];
}

const typeIcons = {
  agent: Bot,
  promptpack: FileText,
  toolregistry: Package,
};

const typeColors = {
  agent: "bg-blue-500/10 text-blue-600 border-blue-500/20",
  promptpack: "bg-purple-500/10 text-purple-600 border-purple-500/20",
  toolregistry: "bg-orange-500/10 text-orange-600 border-orange-500/20",
};

const typeLabels = {
  agent: "Agent",
  promptpack: "PromptPack",
  toolregistry: "ToolRegistry",
};

// Custom hook to sync with localStorage without hydration issues
function useNotesStore(): NotesMap {
  const getSnapshot = () => {
    if (typeof globalThis.window === "undefined") return "{}";
    return localStorage.getItem("topology-notes") || "{}";
  };

  const getServerSnapshot = () => "{}";

  const subscribe = (callback: () => void) => {
    globalThis.window.addEventListener("storage", callback);
    return () => globalThis.window.removeEventListener("storage", callback);
  };

  const notesString = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  return JSON.parse(notesString) as NotesMap;
}

export function NotesPanel({ resources, selectedNamespaces }: Readonly<NotesPanelProps>) {
  const notes = useNotesStore();
  const [isAdding, setIsAdding] = useState(false);
  const [selectedResource, setSelectedResource] = useState<string>("");
  const [newNote, setNewNote] = useState("");
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editText, setEditText] = useState("");

  // Filter resources by selected namespaces
  const filteredResources = resources.filter(
    (r) => selectedNamespaces.length === 0 || selectedNamespaces.includes(r.namespace)
  );

  // Filter notes by selected namespaces
  const filteredNotes = Object.entries(notes).filter(
    ([, note]) =>
      selectedNamespaces.length === 0 || selectedNamespaces.includes(note.namespace)
  );

  // Helper to trigger re-read from localStorage via storage event
  const triggerRefresh = () => {
    globalThis.window.dispatchEvent(new Event("storage"));
  };

  const handleAddNote = () => {
    if (!selectedResource || !newNote.trim()) return;

    const [type, namespace, name] = selectedResource.split("/") as [
      "agent" | "promptpack" | "toolregistry",
      string,
      string
    ];
    setNote(type, namespace, name, newNote);
    triggerRefresh();
    setNewNote("");
    setSelectedResource("");
    setIsAdding(false);
  };

  const handleUpdateNote = (key: string) => {
    const note = notes[key];
    if (!note) return;
    setNote(note.resourceType, note.namespace, note.name, editText);
    triggerRefresh();
    setEditingKey(null);
    setEditText("");
  };

  const handleDeleteNote = (key: string) => {
    const note = notes[key];
    if (!note) return;
    deleteNote(note.resourceType, note.namespace, note.name);
    triggerRefresh();
  };

  const startEditing = (key: string, currentNote: string) => {
    setEditingKey(key);
    setEditText(currentNote);
  };

  // Resources that don't have notes yet
  const resourcesWithoutNotes = filteredResources.filter(
    (r) => !notes[`${r.type}/${r.namespace}/${r.name}`]
  );

  return (
    <Sheet>
      <SheetTrigger asChild>
        <Button variant="outline" size="sm" className="h-8 gap-2">
          <StickyNote className="h-3.5 w-3.5" />
          <span className="text-xs">Notes ({filteredNotes.length})</span>
        </Button>
      </SheetTrigger>
      <SheetContent className="w-[400px] sm:w-[540px]">
        <SheetHeader>
          <SheetTitle>Topology Notes</SheetTitle>
          <SheetDescription>
            Add notes to resources for documentation and context.
            Notes are stored locally in your browser.
          </SheetDescription>
        </SheetHeader>

        <div className="mt-6 space-y-4">
          {/* Add Note Section */}
          {isAdding ? (
            <div className="space-y-3 p-3 border rounded-lg bg-muted/30">
              <Select value={selectedResource} onValueChange={setSelectedResource}>
                <SelectTrigger>
                  <SelectValue placeholder="Select a resource..." />
                </SelectTrigger>
                <SelectContent>
                  {resourcesWithoutNotes.map((r) => {
                    const Icon = typeIcons[r.type];
                    return (
                      <SelectItem
                        key={`${r.type}/${r.namespace}/${r.name}`}
                        value={`${r.type}/${r.namespace}/${r.name}`}
                      >
                        <div className="flex items-center gap-2">
                          <Icon className="h-3.5 w-3.5" />
                          <span>{r.name}</span>
                          <span className="text-muted-foreground text-xs">
                            ({r.namespace})
                          </span>
                        </div>
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>
              <Textarea
                placeholder="Enter your note..."
                value={newNote}
                onChange={(e) => setNewNote(e.target.value)}
                rows={3}
              />
              <div className="flex justify-end gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setIsAdding(false);
                    setNewNote("");
                    setSelectedResource("");
                  }}
                >
                  Cancel
                </Button>
                <Button
                  size="sm"
                  onClick={handleAddNote}
                  disabled={!selectedResource || !newNote.trim()}
                >
                  Add Note
                </Button>
              </div>
            </div>
          ) : (
            <Button
              variant="outline"
              className="w-full"
              onClick={() => setIsAdding(true)}
              disabled={resourcesWithoutNotes.length === 0}
            >
              <Plus className="h-4 w-4 mr-2" />
              Add Note
            </Button>
          )}

          {/* Notes List */}
          <ScrollArea className="h-[calc(100vh-280px)]">
            <div className="space-y-3 pr-4">
              {filteredNotes.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-8">
                  No notes yet. Add one to get started.
                </p>
              ) : (
                filteredNotes.map(([key, note]) => {
                  const Icon = typeIcons[note.resourceType];
                  const isEditing = editingKey === key;

                  return (
                    <div
                      key={key}
                      className="p-3 border rounded-lg space-y-2 bg-card"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex items-center gap-2 flex-wrap">
                          <Badge
                            variant="outline"
                            className={typeColors[note.resourceType]}
                          >
                            <Icon className="h-3 w-3 mr-1" />
                            {typeLabels[note.resourceType]}
                          </Badge>
                          <span className="font-medium text-sm">{note.name}</span>
                          <span className="text-xs text-muted-foreground">
                            {note.namespace}
                          </span>
                        </div>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-muted-foreground hover:text-destructive"
                          onClick={() => handleDeleteNote(key)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>

                      {isEditing ? (
                        <div className="space-y-2">
                          <Textarea
                            value={editText}
                            onChange={(e) => setEditText(e.target.value)}
                            rows={3}
                          />
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => {
                                setEditingKey(null);
                                setEditText("");
                              }}
                            >
                              Cancel
                            </Button>
                            <Button
                              size="sm"
                              onClick={() => handleUpdateNote(key)}
                            >
                              Save
                            </Button>
                          </div>
                        </div>
                      ) : (
                        <button
                          type="button"
                          className="text-sm text-muted-foreground cursor-pointer hover:text-foreground transition-colors text-left w-full bg-transparent border-none p-0"
                          onClick={() => startEditing(key, note.note)}
                        >
                          {note.note}
                        </button>
                      )}

                      <p className="text-xs text-muted-foreground">
                        Updated {new Date(note.updatedAt).toLocaleDateString()}
                      </p>
                    </div>
                  );
                })
              )}
            </div>
          </ScrollArea>
        </div>
      </SheetContent>
    </Sheet>
  );
}
