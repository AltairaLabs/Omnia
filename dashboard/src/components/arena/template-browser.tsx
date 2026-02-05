"use client";

import { useState, useMemo } from "react";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import { TemplateCard } from "./template-card";
import {
  Search,
  RefreshCw,
  LayoutGrid,
  List,
  AlertCircle,
  Loader2,
  FolderOpen,
} from "lucide-react";
import {
  filterTemplates,
  getTemplateCategories,
  getTemplateTags,
  type ArenaTemplateSource,
  type TemplateMetadata,
} from "@/types/arena-template";

/** Template metadata enriched with source name */
type TemplateWithSource = TemplateMetadata & { sourceName: string };

export interface TemplateBrowserProps {
  /** Templates to display (fetched from API) */
  readonly templates: TemplateWithSource[];
  /** Sources for calculating stats */
  readonly sources: ArenaTemplateSource[];
  readonly loading?: boolean;
  readonly error?: Error | null;
  readonly onRefetch?: () => void;
  readonly onSelectTemplate?: (template: TemplateMetadata, sourceName: string) => void;
  readonly className?: string;
}

/**
 * Template browser component with filtering and search.
 */
// eslint-disable-next-line sonarjs/cognitive-complexity
export function TemplateBrowser({
  templates: allTemplates,
  sources,
  loading,
  error,
  onRefetch,
  onSelectTemplate,
  className,
}: TemplateBrowserProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");

  // Get unique categories and tags
  const categories = useMemo(
    () => getTemplateCategories(allTemplates),
    [allTemplates]
  );
  const availableTags = useMemo(
    () => getTemplateTags(allTemplates),
    [allTemplates]
  );

  // Filter templates based on current filters
  const filteredTemplates = useMemo((): TemplateWithSource[] => {
    return filterTemplates(allTemplates, {
      category: selectedCategory === "all" ? undefined : selectedCategory,
      tags: selectedTags.length > 0 ? selectedTags : undefined,
      search: searchQuery || undefined,
    }) as TemplateWithSource[];
  }, [allTemplates, selectedCategory, selectedTags, searchQuery]);

  // Calculate source stats
  const sourceStats = useMemo(() => {
    const ready = sources.filter((s) => s.status?.phase === "Ready").length;
    const pending = sources.filter((s) => s.status?.phase === "Pending" || s.status?.phase === "Fetching").length;
    const errored = sources.filter((s) => s.status?.phase === "Error").length;
    return { ready, pending, errored, total: sources.length };
  }, [sources]);

  const handleTagToggle = (tag: string) => {
    setSelectedTags((prev) =>
      prev.includes(tag)
        ? prev.filter((t) => t !== tag)
        : [...prev, tag]
    );
  };

  // Helper to render the content section (avoids nested ternary)
  const renderTemplateContent = () => {
    const gridClassName = cn(
      viewMode === "grid"
        ? "grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4"
        : "space-y-2"
    );

    if (loading && allTemplates.length === 0) {
      return (
        <div className={gridClassName}>
          {Array.from({ length: 8 }).map((_, i) => (
            // eslint-disable-next-line react/no-array-index-key
            <Skeleton key={`skeleton-${i}`} className="h-48 rounded-lg" />
          ))}
        </div>
      );
    }

    if (filteredTemplates.length === 0) {
      return (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <FolderOpen className="h-12 w-12 text-muted-foreground mb-4" />
          <h3 className="font-medium">No templates found</h3>
          <p className="text-sm text-muted-foreground mt-1">
            {searchQuery || selectedTags.length > 0
              ? "Try adjusting your search or filters"
              : "Add a template source to get started"}
          </p>
        </div>
      );
    }

    return (
      <div className={gridClassName}>
        {filteredTemplates.map((template) => (
          <TemplateCard
            key={`${template.sourceName}-${template.name}`}
            template={template}
            sourceName={template.sourceName}
            onSelect={() => onSelectTemplate?.(template, template.sourceName)}
            onUse={() => onSelectTemplate?.(template, template.sourceName)}
            className={viewMode === "list" ? "flex-row" : undefined}
          />
        ))}
      </div>
    );
  };

  if (error) {
    return (
      <Alert variant="destructive" className={className}>
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          Failed to load templates: {error.message}
          {onRefetch && (
            <Button variant="link" className="ml-2 p-0 h-auto" onClick={onRefetch}>
              Retry
            </Button>
          )}
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <div className={cn("space-y-6", className)}>
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold">Templates</h2>
          <p className="text-sm text-muted-foreground">
            {filteredTemplates.length} template{filteredTemplates.length === 1 ? "" : "s"} available
            {sourceStats.pending > 0 && (
              <span className="ml-2">
                ({sourceStats.pending} source{sourceStats.pending === 1 ? "" : "s"} syncing)
              </span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {onRefetch && (
            <Button
              variant="outline"
              size="sm"
              onClick={onRefetch}
              disabled={loading}
            >
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              <span className="ml-2 hidden sm:inline">Refresh</span>
            </Button>
          )}
          <div className="flex items-center border rounded-md">
            <Button
              variant={viewMode === "grid" ? "secondary" : "ghost"}
              size="sm"
              className="rounded-r-none"
              onClick={() => setViewMode("grid")}
            >
              <LayoutGrid className="h-4 w-4" />
            </Button>
            <Button
              variant={viewMode === "list" ? "secondary" : "ghost"}
              size="sm"
              className="rounded-l-none"
              onClick={() => setViewMode("list")}
            >
              <List className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>

      {/* Search and filters */}
      <div className="space-y-4">
        {/* Search */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search templates..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>

        {/* Category tabs */}
        {categories.length > 0 && (
          <Tabs value={selectedCategory} onValueChange={setSelectedCategory}>
            <TabsList className="flex-wrap h-auto p-1">
              <TabsTrigger value="all" className="text-xs">
                All
              </TabsTrigger>
              {categories.map((category) => (
                <TabsTrigger key={category} value={category} className="text-xs capitalize">
                  {category}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        )}

        {/* Tag filters */}
        {availableTags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {availableTags.slice(0, 10).map((tag) => (
              <Badge
                key={tag}
                variant={selectedTags.includes(tag) ? "default" : "outline"}
                className="cursor-pointer text-xs"
                onClick={() => handleTagToggle(tag)}
              >
                {tag}
              </Badge>
            ))}
            {availableTags.length > 10 && (
              <Badge variant="outline" className="text-xs">
                +{availableTags.length - 10} more
              </Badge>
            )}
          </div>
        )}
      </div>

      {/* Template grid/list */}
      {renderTemplateContent()}
    </div>
  );
}
