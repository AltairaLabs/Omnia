---
title: "Use the Arena Project Editor"
description: "Create, edit, and test Arena projects with the built-in Project Editor"
sidebar:
  order: 13
  badge:
    text: Arena
    variant: note
---

:::note[Enterprise Feature]
The Project Editor is an enterprise feature. Enable it with `enterprise.enabled=true` in your Helm values. See [Installing a License](/how-to/install-license/) for details.
:::

The Project Editor provides an in-browser development environment for creating and testing Arena projects. It includes a Monaco-based YAML editor, file management, real-time validation, and interactive agent testing.

## Prerequisites

- Enterprise features enabled (`enterprise.enabled=true`)
- PromptKit LSP enabled for validation (`enterprise.promptkitLsp.enabled=true`)
- A workspace with Editor or Admin role

## Accessing the Project Editor

1. Open the Omnia Dashboard
2. Navigate to your workspace
3. Go to **Arena** > **Projects**
4. Click **New Project** or select an existing project

## Creating a New Project

### From a Template

The easiest way to start is from a template:

1. Click **New Project**
2. Select **From Template**
3. Browse available templates by category
4. Select a template and click **Use Template**
5. Fill in the template variables:
   - **Project Name**: A unique name for your project
   - **Custom Variables**: Template-specific settings
6. Click **Create Project**

The template will be rendered with your variables and saved to the workspace.

### From Scratch

To create an empty project:

1. Click **New Project**
2. Select **Blank Project**
3. Enter a project name
4. Click **Create**

You'll start with a minimal project structure that you can build upon.

## Project Editor Interface

### File Tree

The left sidebar shows your project files:

- **Single-click** a file to open it in the editor
- **Right-click** for context menu options:
  - Create new file or folder
  - Rename or delete
  - Import provider or tool configuration

### Monaco Editor

The central editor provides:

- **Syntax highlighting** for YAML, JSON, and Markdown
- **Auto-completion** for PromptKit configuration
- **Inline validation** with error markers
- **Multiple tabs** for editing several files

### Problems Panel

The bottom panel shows validation issues:

- **Errors** (red): Must be fixed before running
- **Warnings** (yellow): Best practice suggestions
- **Info** (blue): Helpful hints

Click a problem to jump to its location in the editor.

## File Types

Arena projects use the [PromptKit](https://promptkit.altairalabs.ai) format:

### `arena.config.yaml`

Main configuration file defining the agent:

```yaml
name: my-chatbot
version: 1.0.0

provider:
  type: openai
  model: gpt-4

prompts:
  - id: default
    file: prompts/main.yaml

tools:
  - name: get_weather
    type: http
    endpoint: http://weather-api/weather
```

### Prompt Files (`prompts/*.yaml`)

Define system prompts and conversation templates:

```yaml
id: default
name: Main Prompt
version: 1.0.0

system_template: |
  You are a helpful assistant named {{ .agentName }}.
  Be concise and friendly.

user_template: |
  {{ .input }}
```

### Scenario Files (`scenarios/*.yaml`)

Define test cases for evaluation:

```yaml
name: greeting-test
description: Test basic greeting behavior

steps:
  - input: "Hello!"
    assertions:
      - type: contains
        value: "Hello"
      - type: not_contains
        value: "error"
```

See the [PromptKit documentation](https://promptkit.altairalabs.ai/docs/configuration) for the complete configuration reference.

## Importing Resources

### Import a Provider

To use a workspace Provider in your project:

1. Right-click in the file tree
2. Select **Import Provider**
3. Choose from available providers in the workspace
4. The provider configuration will be added to your `arena.config.yaml`

### Import a Tool Registry

To add tools from a ToolRegistry:

1. Right-click in the file tree
2. Select **Import Tool**
3. Choose from available tool registries
4. Select specific tools to import

## Validation

### Real-Time Validation

When PromptKit LSP is enabled, validation runs as you type:

- Schema validation for all configuration files
- Reference checking (prompts, tools, providers)
- Template variable validation

### Validate All

Click the **Validate** button in the toolbar to run full validation:

1. All files are checked against PromptKit schemas
2. Cross-file references are verified
3. Results appear in the Problems panel

### Common Validation Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `Unknown property` | Typo or invalid field | Check PromptKit schema |
| `Missing required field` | Required field not set | Add the required field |
| `Invalid reference` | Referenced resource not found | Check file path or ID |
| `Type mismatch` | Wrong value type | Use correct type (string, number, etc.) |

## Testing Your Agent

### Interactive Testing (Dev Console)

Test your agent in real-time with the Dev Console:

1. Click **Test Agent** in the toolbar
2. Select a provider from your workspace
3. Wait for the dev console to initialize
4. Type messages to chat with your agent
5. View tool calls and responses in real-time

The Dev Console supports:
- **Hot reload**: Changes are applied without disconnecting
- **Tool visibility**: See tool calls and results
- **File attachments**: Test multi-modal capabilities
- **Provider switching**: Try different providers

### Running Evaluations

To run automated tests:

1. Click **Deploy** to create an ArenaSource from your project
2. Click **Run** and select a job type:
   - **Evaluation**: Run scenario tests
   - **Load Test**: Performance testing
   - **Data Generation**: Generate synthetic data
3. View results in the Results panel

## Saving and Deploying

### Save Changes

Changes are auto-saved, but you can also:

- Press `Ctrl+S` / `Cmd+S` to save immediately
- Click **Save** in the toolbar

### Deploy to ArenaSource

To deploy your project for ArenaJob execution:

1. Ensure all validation errors are fixed
2. Click **Deploy** in the toolbar
3. The project is packaged and deployed as an ArenaSource

### Check Deployment Status

After deploying:

1. The Deploy button shows deployment status
2. Green indicator = successfully deployed
3. Click to view ArenaSource details

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl/Cmd + S` | Save file |
| `Ctrl/Cmd + P` | Quick open file |
| `Ctrl/Cmd + Shift + P` | Command palette |
| `Ctrl/Cmd + /` | Toggle comment |
| `F2` | Rename symbol |
| `Ctrl/Cmd + .` | Quick fix |
| `Ctrl/Cmd + Space` | Trigger suggestions |

## Best Practices

### Project Organization

```
my-project/
├── arena.config.yaml    # Main configuration
├── prompts/
│   ├── main.yaml        # Primary prompt
│   └── error.yaml       # Error handling prompt
├── scenarios/
│   ├── happy-path.yaml  # Success scenarios
│   └── edge-cases.yaml  # Edge case tests
└── README.md            # Project documentation
```

### Version Control

Projects are stored in the workspace filesystem. Consider:

- Using descriptive project names
- Documenting changes in README.md
- Exporting projects for external version control

### Testing Strategy

1. **Start with the Dev Console**: Quick iteration on prompts
2. **Add scenario tests**: Capture expected behaviors
3. **Run evaluations**: Automated testing across scenarios
4. **Monitor results**: Track quality over time

## Troubleshooting

### Editor Not Loading

If the editor doesn't load:

1. Check browser console for errors
2. Verify enterprise features are enabled
3. Ensure you have workspace access

### Validation Not Working

If validation is missing:

1. Check that `promptkitLsp.enabled=true` in Helm values
2. Verify the LSP service is running:
   ```bash
   kubectl get pods -l app=promptkit-lsp
   ```
3. Check LSP logs for errors

### Dev Console Not Connecting

If the Test Agent feature fails:

1. Verify the workspace has at least one Provider
2. Check ArenaDevSession was created:
   ```bash
   kubectl get arenadevsession -n <workspace-ns>
   ```
3. Check dev console pod logs

### Deploy Failing

If deployment fails:

1. Fix all validation errors first
2. Check workspace filesystem permissions
3. Verify ArenaSource controller is running

## Related Resources

- **[ArenaTemplateSource CRD](/reference/arena-template-source)**: Template source configuration
- **[ArenaDevSession CRD](/reference/arena-dev-session)**: Interactive testing sessions
- **[Monitor Arena Jobs](/how-to/monitor-arena-jobs)**: Track evaluation progress
- **[PromptKit Documentation](https://promptkit.altairalabs.ai)**: Configuration reference
