"use client";

import { useQueryClient } from "@tanstack/react-query";
import { EditableConfigPanel } from "@/components/resources/editable-config-panel";
import { useToolRegistryMutations } from "@/hooks/use-tool-registry-mutations";
import type {
  EditableResource,
  UpdateResourceBody,
} from "@/components/resources/editable-config-panel.utils";
import type { ToolRegistry } from "@/types/tool-registry";

export function ToolRegistryConfigTab({ registry }: { readonly registry: ToolRegistry }) {
  const { updateToolRegistry } = useToolRegistryMutations();
  const queryClient = useQueryClient();
  const name = registry.metadata?.name ?? "";

  const onSave = async (body: UpdateResourceBody): Promise<EditableResource> => {
    const saved = await updateToolRegistry(name, body);
    await queryClient.invalidateQueries({ queryKey: ["toolRegistry"] });
    return saved as unknown as EditableResource;
  };

  return (
    <EditableConfigPanel
      kind="ToolRegistry"
      name={name}
      resource={registry as unknown as EditableResource}
      onSave={onSave}
    />
  );
}
