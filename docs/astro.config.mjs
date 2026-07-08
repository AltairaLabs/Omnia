// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import d2 from 'astro-d2';

// Support building archived versions at a subpath (e.g., /v0-2/)
const basePath = process.env.BASE_PATH || '/';

// https://astro.build/config
export default defineConfig({
  site: 'https://omnia.altairalabs.ai',
  base: basePath,
  // Redirects from the pre-taxonomy flat URLs to the new domain-grouped paths.
  redirects: {
    '/how-to/expose-agents/': '/how-to/agents/expose-agents/',
    '/how-to/scale-agents/': '/how-to/agents/scale-agents/',
    '/how-to/configure-sessions/': '/how-to/agents/configure-sessions/',
    '/how-to/define-functions/': '/how-to/agents/define-functions/',
    '/how-to/expose-functions-as-mcp/': '/how-to/agents/expose-functions-as-mcp/',
    '/how-to/use-skills/': '/how-to/agents/use-skills/',
    '/how-to/configure-azure-ai-provider/': '/how-to/providers/configure-azure-ai-provider/',
    '/how-to/configure-bedrock-provider/': '/how-to/providers/configure-bedrock-provider/',
    '/how-to/configure-openrouter-provider/': '/how-to/providers/configure-openrouter-provider/',
    '/how-to/configure-vertex-provider/': '/how-to/providers/configure-vertex-provider/',
    '/how-to/change-memory-embedding-model/': '/how-to/memory/change-memory-embedding-model/',
    '/how-to/configure-memory-consolidation/': '/how-to/memory/configure-memory-consolidation/',
    '/how-to/configure-arena-s3-storage/': '/how-to/evaluation/configure-arena-s3-storage/',
    '/how-to/monitor-arena-jobs/': '/how-to/evaluation/monitor-arena-jobs/',
    '/how-to/setup-arena-scheduled-jobs/': '/how-to/evaluation/setup-arena-scheduled-jobs/',
    '/how-to/troubleshoot-arena/': '/how-to/evaluation/troubleshoot-arena/',
    '/how-to/use-arena-project-editor/': '/how-to/evaluation/use-arena-project-editor/',
    '/how-to/configure-realtime-evals/': '/how-to/evaluation/configure-realtime-evals/',
    '/how-to/setup-observability/': '/how-to/observability/setup-observability/',
    '/how-to/configure-otlp-ingestion/': '/how-to/observability/configure-otlp-ingestion/',
    '/how-to/configure-authentication/': '/how-to/security/configure-authentication/',
    '/how-to/configure-dashboard-auth/': '/how-to/security/configure-dashboard-auth/',
    '/how-to/configure-agent-policies/': '/how-to/security/configure-agent-policies/',
    '/how-to/configure-tool-policies/': '/how-to/security/configure-tool-policies/',
    '/how-to/api-keys/': '/how-to/security/api-keys/',
    '/how-to/configure-privacy-policies/': '/how-to/privacy/configure-privacy-policies/',
    '/how-to/handle-data-subject-erasure/': '/how-to/privacy/handle-data-subject-erasure/',
    '/how-to/manage-workspaces/': '/how-to/workspaces/manage-workspaces/',
    '/how-to/isolate-workspace-content/': '/how-to/workspaces/isolate-workspace-content/',
    '/how-to/install-license/': '/how-to/operations/install-license/',
    '/how-to/configure-pod-overrides/': '/how-to/operations/configure-pod-overrides/',
    '/how-to/configure-media-storage/': '/how-to/operations/configure-media-storage/',
    '/how-to/white-label-the-dashboard/': '/how-to/operations/white-label-the-dashboard/',
    '/how-to/query-session-archive/': '/how-to/operations/query-session-archive/',
    '/how-to/local-development/': '/how-to/operations/local-development/',
    '/reference/agentruntime/': '/reference/core/agentruntime/',
    '/reference/promptpack/': '/reference/core/promptpack/',
    '/reference/provider/': '/reference/core/provider/',
    '/reference/toolregistry/': '/reference/core/toolregistry/',
    '/reference/skillsource/': '/reference/core/skillsource/',
    '/reference/workspace/': '/reference/core/workspace/',
    '/reference/agentpolicy/': '/reference/policies/agentpolicy/',
    '/reference/toolpolicy/': '/reference/policies/toolpolicy/',
    '/reference/sessionprivacypolicy/': '/reference/policies/sessionprivacypolicy/',
    '/reference/arenasource/': '/reference/evaluation/arenasource/',
    '/reference/arenaconfig/': '/reference/evaluation/arenaconfig/',
    '/reference/arenajob/': '/reference/evaluation/arenajob/',
    '/reference/arena-dev-session/': '/reference/evaluation/arena-dev-session/',
    '/reference/arena-template-source/': '/reference/evaluation/arena-template-source/',
    '/reference/helm-values/': '/reference/platform/helm-values/',
    '/reference/dashboard-auth/': '/reference/platform/dashboard-auth/',
    '/reference/websocket-protocol/': '/reference/platform/websocket-protocol/',
    '/explanation/architecture/': '/explanation/platform/architecture/',
    '/explanation/reconciliation/': '/explanation/platform/reconciliation/',
    '/explanation/multi-tenancy/': '/explanation/platform/multi-tenancy/',
    '/explanation/licensing/': '/explanation/platform/licensing/',
    '/explanation/autoscaling/': '/explanation/agents/autoscaling/',
    '/explanation/rollout-strategies/': '/explanation/agents/rollout-strategies/',
    '/explanation/sessions/': '/explanation/agents/sessions/',
    '/explanation/authentication/': '/explanation/security/authentication/',
    '/explanation/policy-engine/': '/explanation/security/policy-engine/',
    '/explanation/arena-fleet/': '/explanation/evaluation/arena-fleet/',
    '/explanation/realtime-evals/': '/explanation/evaluation/realtime-evals/',
  },
  integrations: [
    // Pass `output` explicitly: astro-d2 0.8.x stopped honouring its
    // Zod default when no userConfig is supplied, so config.output
    // arrives as undefined and the build crashes on path.join.
    d2({ output: 'd2' }),
    starlight({
      title: 'Omnia',
      description: 'Kubernetes operator for managing AI agent deployments',
      // Atlas design system (fonts + tokens + doc-chrome reskin).
      customCss: ['./src/styles/custom.css'],
      // Code blocks: inky Atlas surfaces + starlight/cyan-leaning syntax
      // (poimandres for the night sky, a light theme for the printed chart).
      expressiveCode: {
        themes: ['poimandres', 'github-light'],
        styleOverrides: {
          borderColor: 'var(--hairline)',
          borderRadius: 'var(--radius-code)',
          codeBackground: 'var(--surface-code)',
          frames: {
            editorBackground: 'var(--surface-code)',
            terminalBackground: 'var(--surface-code)',
            editorTabBarBackground: 'var(--ink-surface)',
            editorActiveTabBackground: 'var(--surface-code)',
            terminalTitlebarBackground: 'var(--ink-surface)',
          },
        },
      },
      logo: {
        src: './public/atlas/logo-omnia.svg',
        alt: 'Omnia',
        replacesTitle: false,
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AltairaLabs/Omnia' },
      ],
      // Header adds the AltairaLabs family bar; SocialIcons + PageTitle carry the
      // version switcher and "not latest" banner.
      components: {
        Header: './src/components/Header.astro',
        SocialIcons: './src/components/SocialIcons.astro',
        PageTitle: './src/components/PageTitle.astro',
      },
      sidebar: [
      {
        label: 'Tutorials',
        items: [{ autogenerate: { directory: 'tutorials' } }],
      },
      {
        label: "How-To Guides",
        items: [
        { label: "Agents", collapsed: true, items: [{ autogenerate: { directory: "how-to/agents" } }] },
        { label: "Tools", collapsed: true, items: [{ autogenerate: { directory: "how-to/tools" } }] },
        { label: "Providers", collapsed: true, items: [{ autogenerate: { directory: "how-to/providers" } }] },
        { label: "Memory", collapsed: true, items: [{ autogenerate: { directory: "how-to/memory" } }] },
        { label: "Observability", collapsed: true, items: [{ autogenerate: { directory: "how-to/observability" } }] },
        { label: "Security & access", collapsed: true, items: [{ autogenerate: { directory: "how-to/security" } }] },
        { label: "Privacy & compliance", collapsed: true, items: [{ autogenerate: { directory: "how-to/privacy" } }] },
        { label: "Workspaces", collapsed: true, items: [{ autogenerate: { directory: "how-to/workspaces" } }] },
        { label: "Testing & evaluation", collapsed: true, items: [{ autogenerate: { directory: "how-to/evaluation" } }] },
        { label: "Platform & operations", collapsed: true, items: [{ autogenerate: { directory: "how-to/operations" } }] },
        ],
      },
      {
        label: "Reference",
        items: [
        { label: "Core CRDs", collapsed: true, items: [{ autogenerate: { directory: "reference/core" } }] },
        { label: "Policy CRDs", collapsed: true, items: [{ autogenerate: { directory: "reference/policies" } }] },
        { label: "Platform & protocol", collapsed: true, items: [{ autogenerate: { directory: "reference/platform" } }] },
        { label: "Arena CRDs", collapsed: true, items: [{ autogenerate: { directory: "reference/evaluation" } }] },
        ],
      },
      {
        label: "Concepts",
        items: [
        { label: "Platform", collapsed: true, items: [{ autogenerate: { directory: "explanation/platform" } }] },
        { label: "Agents", collapsed: true, items: [{ autogenerate: { directory: "explanation/agents" } }] },
        { label: "Security", collapsed: true, items: [{ autogenerate: { directory: "explanation/security" } }] },
        { label: "Testing & evaluation", collapsed: true, items: [{ autogenerate: { directory: "explanation/evaluation" } }] },
        ],
      },
      ],
      editLink: {
        baseUrl: 'https://github.com/AltairaLabs/Omnia/edit/main/docs/',
      },
      head: [
        {
          tag: 'script',
          attrs: {
            type: 'module',
            src: '/mermaid-init.js',
          },
        },
      ],
    }),
  ],
});
