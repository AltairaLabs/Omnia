"use client";

import { Header } from "@/components/layout";
import { ConsoleTabs } from "@/components/console";

export default function ConsolePage() {
  return (
    <div className="flex flex-col h-full">
      <Header
        title="Console"
        description="Interactive agent sessions"
      />
      <div className="flex-1 p-6 overflow-hidden">
        <ConsoleTabs />
      </div>
    </div>
  );
}
