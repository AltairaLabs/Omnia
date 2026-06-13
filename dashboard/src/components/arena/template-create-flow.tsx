"use client";

import { useState, useCallback } from "react";
import { TemplateBrowser } from "./template-browser";
import { TemplateWizard, type ProviderOption } from "./template-wizard";
import { useProviders } from "@/hooks/resources";
import { useTemplateSources, useAllTemplates, useTemplateRendering } from "@/hooks/arena";
import type { TemplateMetadata, TemplateRenderInput } from "@/types/arena-template";

interface TemplateCreateFlowProps {
  /** Called with the new project id after the wizard creates a project. */
  readonly onSuccess: (projectId: string) => void;
  readonly className?: string;
}

/**
 * Shared browse -> configure -> create flow over the existing TemplateBrowser
 * and TemplateWizard. Used by the Templates page and by the project editor's
 * "New from template" dialog so both reuse the exact same wizard.
 */
export function TemplateCreateFlow({ onSuccess, className }: TemplateCreateFlowProps) {
  const { sources, loading: sourcesLoading, error: sourcesError, refetch: refetchSources } = useTemplateSources();
  const { templates, loading: templatesLoading, error, refetch: refetchTemplates } = useAllTemplates();
  const { preview, render } = useTemplateRendering();
  const { data: providers = [] } = useProviders({ phase: "Ready" });

  const loading = sourcesLoading || templatesLoading;

  // Providers need a model to be selectable in the wizard.
  const providerOptions: ProviderOption[] = providers
    .filter((p) => p.spec?.model)
    .map((p) => ({ name: p.metadata.name, model: p.spec!.model!, displayName: p.metadata.name }));

  const [selected, setSelected] = useState<{ template: TemplateMetadata; sourceName: string } | null>(null);

  const refetch = useCallback(() => {
    refetchSources();
    refetchTemplates();
  }, [refetchSources, refetchTemplates]);

  const handleSelectTemplate = useCallback((template: TemplateMetadata, sourceName: string) => {
    setSelected({ template, sourceName });
  }, []);

  const handlePreview = useCallback(
    async (input: Omit<TemplateRenderInput, "projectName">) => {
      if (!selected) throw new Error("No template selected");
      return preview(selected.sourceName, selected.template.name, input);
    },
    [selected, preview],
  );

  const handleSubmit = useCallback(
    async (input: TemplateRenderInput) => {
      if (!selected) throw new Error("No template selected");
      return render(selected.sourceName, selected.template.name, input);
    },
    [selected, render],
  );

  if (selected) {
    return (
      <TemplateWizard
        template={selected.template}
        sourceName={selected.sourceName}
        providers={providerOptions}
        onPreview={handlePreview}
        onSubmit={handleSubmit}
        onSuccess={onSuccess}
        onClose={() => setSelected(null)}
        className={className}
      />
    );
  }

  return (
    <TemplateBrowser
      templates={templates}
      sources={sources}
      loading={loading}
      error={error ?? sourcesError}
      onRefetch={refetch}
      onSelectTemplate={handleSelectTemplate}
      className={className}
    />
  );
}
