"use client";

import { Header } from "@/components/layout";
import { EnterpriseGate } from "@/components/license/license-gate";
import { InstitutionalKnowledgePanel } from "@/components/memories/institutional-knowledge-panel";

export default function WorkspaceKnowledgePage() {
  return (
    <div className="flex flex-col h-full">
      <Header
        title="Workspace knowledge"
        description="Shared facts, policies, and glossaries every agent in this workspace can use."
      />
      <div className="flex-1 overflow-auto p-6">
        <EnterpriseGate featureName="Institutional knowledge">
          <InstitutionalKnowledgePanel />
        </EnterpriseGate>
      </div>
    </div>
  );
}
