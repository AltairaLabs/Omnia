---
title: "Architecture Overview"
description: "Understanding Omnia's architecture and design decisions"
order: 1
---

# Architecture Overview

This document explains the architecture of Omnia and the design decisions behind it.

## High-Level Architecture

Omnia consists of three main components:

```
┌─────────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│  │   Omnia     │    │   Agent     │    │   Agent     │        │
│  │  Operator   │───▶│   Pod 1     │    │   Pod 2     │        │
│  └─────────────┘    └─────────────┘    └─────────────┘        │
│         │                  │                  │                 │
│         │           ┌──────┴──────────────────┘                │
│         │           │                                          │
│         ▼           ▼                                          │
│  ┌─────────────┐  ┌─────────────┐                              │
│  │  PromptPack │  │    Redis    │                              │
│  │  ConfigMap  │  │  (Sessions) │                              │
│  └─────────────┘  └─────────────┘                              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
         │
         │ WebSocket
         ▼
    ┌─────────┐
    │ Clients │
    └─────────┘
```

## Components

### Omnia Operator

The operator is a Kubernetes controller that:

- Watches for AgentRuntime, PromptPack, and ToolRegistry resources
- Creates and manages Deployments for agent pods
- Creates Services for agent access
- Monitors referenced resources and updates agents accordingly

The operator follows the standard Kubernetes controller pattern:

1. **Watch** - Monitor custom resources for changes
2. **Reconcile** - Bring actual state to desired state
3. **Status** - Report current state back to the resource

### Agent Container

Each agent pod runs the Omnia agent container, which provides:

- **WebSocket Facade** - Handles client connections and message routing
- **Session Management** - Maintains conversation state
- **LLM Integration** - Communicates with configured providers
- **Tool Execution** - Invokes tools from the ToolRegistry

### Custom Resource Definitions

#### AgentRuntime

The primary resource for deploying agents. It references:

- Provider configuration (which LLM to use)
- PromptPack (what prompts to use)
- ToolRegistry (what tools are available)
- Session configuration

#### PromptPack

Defines versioned prompt configurations following the [PromptPack specification](https://promptpack.org/docs/spec/schema-reference). Supports:

- Structured prompt definitions with variables, parameters, and validators
- ConfigMap-based storage of compiled PromptPack JSON
- Canary rollouts for safe prompt updates
- Automatic agent notification on changes

#### ToolRegistry

Defines tools available to agents:

- Inline URL-based tools
- Service discovery via label selectors
- Mixed sources in a single registry

## Design Decisions

### Why Kubernetes Operator?

We chose the operator pattern because:

1. **Native integration** - Agents are first-class Kubernetes citizens
2. **Declarative configuration** - Define desired state, not procedures
3. **Self-healing** - Automatic recovery from failures
4. **Scalability** - Leverage Kubernetes scaling mechanisms

### Why WebSocket?

WebSocket was chosen for the client facade because:

1. **Streaming** - Essential for LLM response streaming
2. **Bidirectional** - Enables tool calls and results
3. **Persistent** - Maintains connection for multi-turn conversations
4. **Efficient** - Lower overhead than HTTP polling

### Why Separate PromptPack?

Separating prompts from agents allows:

1. **Reusability** - Same prompts across multiple agents
2. **Versioning** - Track prompt changes independently
3. **Safe rollouts** - Canary deployments for prompts
4. **Separation of concerns** - Prompt engineers vs DevOps

### Why Service Discovery for Tools?

Label-based tool discovery enables:

1. **Dynamic registration** - Tools can come and go
2. **Team boundaries** - Teams own their tool services
3. **Decoupling** - Agents don't need to know tool details
4. **Standard Kubernetes** - Uses familiar patterns

## Resource Relationships

```
AgentRuntime
    │
    ├── references ──▶ PromptPack ──▶ ConfigMap
    │
    ├── references ──▶ ToolRegistry ──▶ Services (via selector)
    │
    ├── creates ────▶ Deployment
    │
    └── creates ────▶ Service
```

## Reconciliation Flow

When an AgentRuntime is created or updated:

1. Validate the referenced PromptPack exists
2. Optionally validate the referenced ToolRegistry
3. Build the agent container spec with environment variables
4. Create or update the Deployment
5. Create or update the Service
6. Update the AgentRuntime status

When a PromptPack changes:

1. Validate the source ConfigMap
2. Find all AgentRuntimes referencing this PromptPack
3. Trigger reconciliation for those agents
4. Update PromptPack status with rollout state

## Security Considerations

### Secrets Management

- API keys are stored in Kubernetes Secrets
- Secrets are mounted as environment variables, not files
- Secrets can be from the same or different namespace

### Network Policies

Consider implementing NetworkPolicies to:

- Restrict agent egress to allowed LLM providers
- Limit tool access to specific services
- Isolate agent namespaces

### RBAC

The operator requires specific permissions:

- Full access to Omnia CRDs
- Read access to ConfigMaps and Secrets
- Create/Update access to Deployments and Services
