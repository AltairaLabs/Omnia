"use client";

import { Header } from "@/components/layout";
import { EnterpriseGate } from "@/components/license/license-gate";
import { ProjectEditor } from "@/components/arena";

function ProjectsContent() {
  return (
    <div className="flex flex-col h-full">
      <Header
        title="Project Editor"
        description="Create and edit Arena project configurations"
      />
      <div className="flex-1 min-h-0">
        <ProjectEditor />
      </div>
    </div>
  );
}

export default function ArenaProjectsPage() {
  return (
    <EnterpriseGate featureName="Arena Project Editor">
      <ProjectsContent />
    </EnterpriseGate>
  );
}
