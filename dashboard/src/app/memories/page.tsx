"use client";

import { useState, useMemo, type ReactNode } from "react";
import { Header } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Brain, Download, Trash2, Search, AlertCircle, LogIn } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useMemories } from "@/hooks/use-memories";
import {
  useDeleteMemory,
  useDeleteAllMemories,
  useExportMemories,
} from "@/hooks/use-memory-mutations";
import { MemoryGraph } from "@/components/memories/memory-graph";
import { MemoryDetailPanel } from "@/components/memories/memory-detail-panel";
import { ConsentBanner } from "@/components/memories/consent-banner";
import type { MemoryEntity } from "@/lib/data/types";

const CATEGORIES = [
  { value: "all", label: "All Categories" },
  { value: "memory:identity", label: "Identity" },
  { value: "memory:context", label: "Context" },
  { value: "memory:health", label: "Health" },
  { value: "memory:location", label: "Location" },
  { value: "memory:preferences", label: "Preferences" },
  { value: "memory:history", label: "History" },
];

interface MemoriesBodyState {
  isAuthenticated: boolean;
  error: unknown;
  isLoading: boolean;
  filtered: MemoryEntity[];
  onSelect: (memory: MemoryEntity) => void;
}

function renderMemoriesBody({
  isAuthenticated,
  error,
  isLoading,
  filtered,
  onSelect,
}: MemoriesBodyState): ReactNode {
  if (!isAuthenticated) {
    return (
      <Alert data-testid="memory-anonymous-notice">
        <LogIn className="h-4 w-4" />
        <AlertTitle>Memories require sign-in</AlertTitle>
        <AlertDescription>
          You&apos;re viewing as an anonymous user. Memories are scoped to
          authenticated identities so each user&apos;s data stays private. Sign
          in to start saving memories.
        </AlertDescription>
      </Alert>
    );
  }
  if (error) {
    return (
      <Alert variant="destructive" data-testid="memory-error">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Could not load memories</AlertTitle>
        <AlertDescription>
          {error instanceof Error
            ? error.message
            : "Failed to connect to the Memory API. Check that the service is running."}
        </AlertDescription>
      </Alert>
    );
  }
  if (isLoading) {
    return <Skeleton className="w-full h-[600px] rounded-lg" />;
  }
  if (filtered.length === 0) {
    return (
      <div
        className="flex flex-col items-center justify-center h-[400px] text-muted-foreground"
        data-testid="empty-state"
      >
        <Brain className="h-16 w-16 mb-4 opacity-30" />
        <h3 className="text-lg font-medium mb-1">No memories yet</h3>
        <p className="text-sm">
          As you interact with agents, they&apos;ll remember things here.
        </p>
      </div>
    );
  }
  return <MemoryGraph memories={filtered} onNodeClick={onSelect} />;
}

export default function MemoriesPage() {
  const { user, isAuthenticated } = useAuth();
  // Always filter by userId — memories belong to users, not workspaces.
  // The proxy hashes the userId before querying (pseudonymous storage).
  // Anonymous users are skipped entirely: the memory-api rejects requests
  // without a user-owned scope, so fetching would just produce an error.
  const { data, isLoading, error } = useMemories({
    userId: user?.id,
    limit: 500,
    enabled: isAuthenticated,
  });
  const [selectedMemory, setSelectedMemory] = useState<MemoryEntity | null>(
    null
  );
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [searchQuery, setSearchQuery] = useState("");

  const deleteMemory = useDeleteMemory();
  const deleteAll = useDeleteAllMemories();
  const exportMemories = useExportMemories();

  const filtered = useMemo(() => {
    let memories = data?.memories ?? [];
    if (categoryFilter !== "all") {
      memories = memories.filter(
        (m) => (m.metadata?.consent_category as string) === categoryFilter
      );
    }
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      memories = memories.filter((m) => m.content.toLowerCase().includes(q));
    }
    return memories;
  }, [data?.memories, categoryFilter, searchQuery]);

  const handleDelete = (memoryId: string) => {
    deleteMemory.mutate(memoryId);
    setSelectedMemory(null);
  };

  const handleForgetAll = () => {
    deleteAll.mutate();
    setSelectedMemory(null);
  };

  return (
    <div className="flex flex-col h-full">
      <Header
        title="My Memories"
        description="What the agent remembers about you"
      />

      <div className="flex-1 overflow-auto p-6 space-y-4">
        <ConsentBanner />

        {isAuthenticated && (
        <div
          className="flex items-center gap-3 flex-wrap"
          data-testid="memories-toolbar"
        >
          <div className="relative flex-1 min-w-[200px] max-w-sm">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search memories..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9"
              data-testid="memory-search"
            />
          </div>

          <Select value={categoryFilter} onValueChange={setCategoryFilter}>
            <SelectTrigger className="w-[180px]" data-testid="category-filter">
              <SelectValue placeholder="Category" />
            </SelectTrigger>
            <SelectContent>
              {CATEGORIES.map((cat) => (
                <SelectItem key={cat.value} value={cat.value}>
                  {cat.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="flex-1" />

          <Button
            variant="outline"
            size="sm"
            onClick={() => exportMemories.mutate()}
            disabled={exportMemories.isPending}
          >
            <Download className="h-4 w-4 mr-2" />
            Export
          </Button>

          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="destructive"
                size="sm"
                data-testid="forget-all-button"
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Forget Everything
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Forget everything?</AlertDialogTitle>
                <AlertDialogDescription>
                  This will permanently delete all your memories across all
                  agents. This cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  onClick={handleForgetAll}
                  data-testid="confirm-forget-all"
                >
                  Forget Everything
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
        )}

        {renderMemoriesBody({
          isAuthenticated,
          error,
          isLoading,
          filtered,
          onSelect: setSelectedMemory,
        })}

        {!isLoading && (data?.total ?? 0) > 0 && (
          <p className="text-xs text-muted-foreground text-center">
            Showing {filtered.length} of {data?.total} memories
          </p>
        )}
      </div>

      <MemoryDetailPanel
        memory={selectedMemory}
        onClose={() => setSelectedMemory(null)}
        onDelete={handleDelete}
      />
    </div>
  );
}
