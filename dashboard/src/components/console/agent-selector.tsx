"use client";

import { useState, useMemo } from "react";
import { Bot, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAgents } from "@/hooks/use-agents";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";

interface AgentSelectorProps {
  /** Called when an agent is selected and confirmed */
  onSelect: (namespace: string, agentName: string) => void;
  /** Optional CSS class name */
  className?: string;
}

function getAgentPlaceholder(isLoading: boolean, agentCount: number): string {
  if (isLoading) return "Loading agents...";
  if (agentCount === 0) return "No running agents";
  return "Select an agent";
}

/**
 * Agent selector component for choosing an agent to start a conversation with.
 * Shows namespace and agent dropdowns with a "Start Conversation" button.
 */
export function AgentSelector({ onSelect, className }: Readonly<AgentSelectorProps>) {
  const [selectedNamespace, setSelectedNamespace] = useState<string>("");
  const [selectedAgent, setSelectedAgent] = useState<string>("");

  // Fetch all running agents (we'll derive namespaces from them)
  const { data: agents, isLoading: agentsLoading } = useAgents({
    phase: "Running",
  });

  // Get namespaces that have running agents (exclude empty namespaces)
  const availableNamespaces = useMemo(() => {
    if (!agents) return [];
    const ns = new Set(agents.map((a) => a.metadata.namespace).filter(Boolean));
    return Array.from(ns).sort() as string[];
  }, [agents]);

  // Filter agents by selected namespace
  const filteredAgents = useMemo(() => {
    if (!agents) return [];
    if (!selectedNamespace) return agents;
    return agents.filter((a) => a.metadata.namespace === selectedNamespace);
  }, [agents, selectedNamespace]);

  const handleNamespaceChange = (value: string) => {
    setSelectedNamespace(value === "__all__" ? "" : value);
    setSelectedAgent(""); // Reset agent when namespace changes
  };

  const handleAgentChange = (value: string) => {
    setSelectedAgent(value);
  };

  const handleStartConversation = () => {
    if (selectedAgent) {
      // Find the agent to get its namespace
      const agent = filteredAgents.find((a) => a.metadata.name === selectedAgent);
      if (agent) {
        onSelect(agent.metadata.namespace || "default", agent.metadata.name);
      }
    }
  };

  const isLoading = agentsLoading;
  const canStart = selectedAgent && !isLoading;

  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center h-full p-8",
        className
      )}
    >
      <div className="flex flex-col items-center gap-6 max-w-md w-full">
        {/* Icon */}
        <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted">
          <Bot className="h-8 w-8 text-muted-foreground" />
        </div>

        {/* Title */}
        <div className="text-center">
          <h2 className="text-lg font-semibold">Start a Conversation</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Select an agent to begin chatting
          </p>
        </div>

        {/* Selectors */}
        <div className="flex flex-col gap-4 w-full">
          {/* Namespace selector */}
          <div className="space-y-2">
            <label htmlFor="namespace-select" className="text-sm font-medium">Namespace</label>
            <Select
              value={selectedNamespace || "__all__"}
              onValueChange={handleNamespaceChange}
              disabled={isLoading}
            >
              <SelectTrigger id="namespace-select">
                <SelectValue placeholder="All namespaces" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">All namespaces</SelectItem>
                {availableNamespaces.map((ns) => (
                  <SelectItem key={ns} value={ns}>
                    {ns}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Agent selector */}
          <div className="space-y-2">
            <label htmlFor="agent-select" className="text-sm font-medium">Agent</label>
            <Select
              value={selectedAgent}
              onValueChange={handleAgentChange}
              disabled={isLoading || filteredAgents.length === 0}
            >
              <SelectTrigger id="agent-select">
                <SelectValue
                  placeholder={getAgentPlaceholder(isLoading, filteredAgents.length)}
                />
              </SelectTrigger>
              <SelectContent>
                {filteredAgents.map((agent) => (
                  <SelectItem key={agent.metadata.uid} value={agent.metadata.name}>
                    <div className="flex items-center gap-2">
                      <span>{agent.metadata.name}</span>
                      {agent.metadata.namespace && selectedNamespace === "" && (
                        <Badge variant="outline" className="text-xs">
                          {agent.metadata.namespace}
                        </Badge>
                      )}
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Start button */}
        <Button
          onClick={handleStartConversation}
          disabled={!canStart}
          className="w-full"
          size="lg"
        >
          {isLoading ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Loading...
            </>
          ) : (
            "Start Conversation"
          )}
        </Button>

        {/* Help text */}
        {filteredAgents.length === 0 && !isLoading && (
          <p className="text-sm text-muted-foreground text-center">
            No running agents available. Deploy an agent first to start a conversation.
          </p>
        )}
      </div>
    </div>
  );
}
