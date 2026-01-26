"use client";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { FileCode, Play, Tag, Folder } from "lucide-react";
import type { TemplateMetadata } from "@/types/arena-template";

export interface TemplateCardProps {
  template: TemplateMetadata;
  sourceName?: string;
  selected?: boolean;
  onSelect?: () => void;
  onUse?: () => void;
  className?: string;
}

/**
 * Get category color based on category name.
 */
function getCategoryColor(category?: string): string {
  switch (category?.toLowerCase()) {
    case "chatbot":
      return "bg-blue-500/10 text-blue-500 border-blue-500/20";
    case "agent":
      return "bg-purple-500/10 text-purple-500 border-purple-500/20";
    case "assistant":
      return "bg-green-500/10 text-green-500 border-green-500/20";
    case "evaluation":
      return "bg-orange-500/10 text-orange-500 border-orange-500/20";
    case "workflow":
      return "bg-pink-500/10 text-pink-500 border-pink-500/20";
    default:
      return "bg-gray-500/10 text-gray-500 border-gray-500/20";
  }
}

/**
 * Template card component for displaying template information.
 */
export function TemplateCard({
  template,
  sourceName,
  selected,
  onSelect,
  onUse,
  className,
}: TemplateCardProps) {
  const displayName = template.displayName || template.name;
  const categoryColor = getCategoryColor(template.category);
  const hasVariables = template.variables && template.variables.length > 0;
  const variableCount = template.variables?.length || 0;

  return (
    <Card
      className={cn(
        "relative cursor-pointer transition-all duration-200 hover:shadow-md",
        selected && "ring-2 ring-primary",
        className
      )}
      onClick={onSelect}
    >
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-2">
          <div className="flex-1 min-w-0">
            <CardTitle className="text-base font-medium truncate">
              {displayName}
            </CardTitle>
            {template.version && (
              <span className="text-xs text-muted-foreground">v{template.version}</span>
            )}
          </div>
          {template.category && (
            <Badge variant="outline" className={cn("text-xs", categoryColor)}>
              {template.category}
            </Badge>
          )}
        </div>
        {template.description && (
          <CardDescription className="text-sm line-clamp-2">
            {template.description}
          </CardDescription>
        )}
      </CardHeader>

      <CardContent className="space-y-3">
        {/* Tags */}
        {template.tags && template.tags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {template.tags.slice(0, 4).map((tag) => (
              <Badge
                key={tag}
                variant="secondary"
                className="text-xs px-1.5 py-0"
              >
                <Tag className="h-3 w-3 mr-1" />
                {tag}
              </Badge>
            ))}
            {template.tags.length > 4 && (
              <Badge variant="secondary" className="text-xs px-1.5 py-0">
                +{template.tags.length - 4}
              </Badge>
            )}
          </div>
        )}

        {/* Info row */}
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          {hasVariables && (
            <div className="flex items-center gap-1">
              <FileCode className="h-3 w-3" />
              <span>{variableCount} variable{variableCount === 1 ? "" : "s"}</span>
            </div>
          )}
          {sourceName && (
            <div className="flex items-center gap-1">
              <Folder className="h-3 w-3" />
              <span className="truncate max-w-[100px]">{sourceName}</span>
            </div>
          )}
        </div>

        {/* Use button */}
        {onUse && (
          <Button
            size="sm"
            className="w-full"
            onClick={(e) => {
              e.stopPropagation();
              onUse();
            }}
          >
            <Play className="h-4 w-4 mr-2" />
            Use Template
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
