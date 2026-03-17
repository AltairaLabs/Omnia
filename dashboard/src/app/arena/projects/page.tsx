"use client";

import dynamic from "next/dynamic";
import { Header } from "@/components/layout";
import { EnterpriseGate } from "@/components/license/license-gate";

const ProjectEditor = dynamic(
  () => import("@/components/arena").then((m) => m.ProjectEditor),
  { ssr: false }
);

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
