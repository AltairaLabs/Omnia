"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
} from "@/components/ui/dialog";
import { EnterpriseGate } from "@/components/license/license-gate";
import { TemplateBrowser } from "@/components/arena/template-browser";
import { TemplateWizard } from "@/components/arena/template-wizard";
import { TemplateSourceDialog } from "@/components/arena/template-source-dialog";
import {
  useTemplateSources,
  useTemplateRendering,
} from "@/hooks/use-template-sources";
import { useToast } from "@/hooks/use-toast";
import {
  Plus,
  RefreshCw,
  Loader2,
  FolderPlus,
} from "lucide-react";
import type { TemplateMetadata } from "@/types/arena-template";

function TemplatesContent() {
  const router = useRouter();
  const { toast } = useToast();
  const { sources, loading, error, refetch } = useTemplateSources();
  const { preview, render } = useTemplateRendering();

  const [sourceDialogOpen, setSourceDialogOpen] = useState(false);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState<{
    template: TemplateMetadata;
    sourceName: string;
  } | null>(null);

  // Count sources by phase
  const sourceStats = {
    ready: sources.filter((s) => s.status?.phase === "Ready").length,
    pending: sources.filter((s) => s.status?.phase === "Fetching" || s.status?.phase === "Pending").length,
    error: sources.filter((s) => s.status?.phase === "Error").length,
    total: sources.length,
  };

  // Count total templates
  const totalTemplates = sources.reduce(
    (sum, s) => sum + (s.status?.templates?.length || 0),
    0
  );

  const handleSelectTemplate = useCallback(
    (template: TemplateMetadata, sourceName: string) => {
      setSelectedTemplate({ template, sourceName });
      setWizardOpen(true);
    },
    []
  );

  const handlePreview = useCallback(
    async (input: { variables: Record<string, string | number | boolean> }) => {
      if (!selectedTemplate) {
        throw new Error("No template selected");
      }
      return preview(selectedTemplate.sourceName, selectedTemplate.template.name, input);
    },
    [selectedTemplate, preview]
  );

  const handleSubmit = useCallback(
    async (input: {
      variables: Record<string, string | number | boolean>;
      projectName: string;
      projectDescription?: string;
      projectTags?: string[];
    }) => {
      if (!selectedTemplate) {
        throw new Error("No template selected");
      }
      const result = await render(
        selectedTemplate.sourceName,
        selectedTemplate.template.name,
        input
      );
      return result;
    },
    [selectedTemplate, render]
  );

  const handleSuccess = useCallback(
    (projectId: string) => {
      setWizardOpen(false);
      setSelectedTemplate(null);
      toast({
        title: "Project created",
        description: `Your project has been created from the template.`,
      });
      // Navigate to the project editor
      router.push(`/arena/projects?id=${projectId}`);
    },
    [router, toast]
  );

  const handleSourceSuccess = useCallback(() => {
    refetch();
    toast({
      title: "Template source added",
      description: "The template source will sync shortly.",
    });
  }, [refetch, toast]);

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Templates"
        description="Browse and use templates to create new Arena projects"
      >
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={loading}
          >
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            <span className="ml-2 hidden sm:inline">Refresh</span>
          </Button>
          <Button onClick={() => setSourceDialogOpen(true)}>
            <Plus className="h-4 w-4 mr-2" />
            Add Source
          </Button>
        </div>
      </Header>

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Stats */}
        <div className="flex items-center gap-4 flex-wrap">
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Sources:</span>
            <Badge variant="secondary">{sourceStats.total}</Badge>
            {sourceStats.ready > 0 && (
              <Badge variant="default" className="bg-green-500">
                {sourceStats.ready} ready
              </Badge>
            )}
            {sourceStats.pending > 0 && (
              <Badge variant="default" className="bg-blue-500">
                {sourceStats.pending} syncing
              </Badge>
            )}
            {sourceStats.error > 0 && (
              <Badge variant="destructive">{sourceStats.error} error</Badge>
            )}
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Templates:</span>
            <Badge variant="secondary">{totalTemplates}</Badge>
          </div>
        </div>

        {/* No sources state */}
        {sources.length === 0 && !loading && (
          <div className="flex flex-col items-center justify-center py-12 text-center border rounded-lg bg-muted/20">
            <FolderPlus className="h-12 w-12 text-muted-foreground mb-4" />
            <h3 className="font-medium text-lg mb-1">No template sources</h3>
            <p className="text-sm text-muted-foreground mb-4 max-w-md">
              Add a template source to start browsing and using templates.
              Template sources can be Git repositories, OCI registries, or ConfigMaps.
            </p>
            <Button onClick={() => setSourceDialogOpen(true)}>
              <Plus className="h-4 w-4 mr-2" />
              Add Template Source
            </Button>
          </div>
        )}

        {/* Template browser */}
        {sources.length > 0 && (
          <TemplateBrowser
            sources={sources}
            loading={loading}
            error={error}
            onRefetch={refetch}
            onSelectTemplate={handleSelectTemplate}
          />
        )}
      </div>

      {/* Add source dialog */}
      <TemplateSourceDialog
        open={sourceDialogOpen}
        onOpenChange={setSourceDialogOpen}
        onSuccess={handleSourceSuccess}
      />

      {/* Template wizard dialog */}
      <Dialog open={wizardOpen} onOpenChange={setWizardOpen}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          {selectedTemplate && (
            <TemplateWizard
              template={selectedTemplate.template}
              sourceName={selectedTemplate.sourceName}
              onPreview={handlePreview}
              onSubmit={handleSubmit}
              onSuccess={handleSuccess}
              onClose={() => {
                setWizardOpen(false);
                setSelectedTemplate(null);
              }}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default function TemplatesPage() {
  return (
    <EnterpriseGate featureName="Arena Fleet">
      <TemplatesContent />
    </EnterpriseGate>
  );
}
