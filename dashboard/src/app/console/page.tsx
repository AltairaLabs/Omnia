"use client";

import dynamic from "next/dynamic";
import { Header } from "@/components/layout";

const ConsoleTabs = dynamic(
  () => import("@/components/console").then((m) => m.ConsoleTabs),
  { ssr: false }
);

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
