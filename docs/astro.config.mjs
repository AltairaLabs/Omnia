// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightThemeGalaxy from 'starlight-theme-galaxy';
import d2 from 'astro-d2';

// Support building archived versions at a subpath (e.g., /v0-2/)
const basePath = process.env.BASE_PATH || '/';

// https://astro.build/config
export default defineConfig({
  site: 'https://omnia.altairalabs.ai',
  base: basePath,
  integrations: [
    // Pass `output` explicitly: astro-d2 0.8.x stopped honouring its
    // Zod default when no userConfig is supplied, so config.output
    // arrives as undefined and the build crashes on path.join.
    d2({ output: 'd2' }),
    starlight({
      plugins: [starlightThemeGalaxy()],
      title: 'Omnia',
      description: 'Kubernetes operator for managing AI agent deployments',
      customCss: [
        './src/styles/accent.css',
        './src/styles/custom.css',
      ],
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
        replacesTitle: false,
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AltairaLabs/Omnia' },
      ],
      // Custom components for version switching
      components: {
        Header: './src/components/Header.astro',
        PageTitle: './src/components/PageTitle.astro',
      },
      sidebar: [
        // Starlight 0.39 dropped top-level { label, autogenerate }
        // shorthand — each group must now use { label, items: [...] }
        // with the autogenerate config as a child entry.
        {
          label: 'Tutorials',
          items: [{ autogenerate: { directory: 'tutorials' } }],
        },
        {
          label: 'How-To Guides',
          items: [{ autogenerate: { directory: 'how-to' } }],
        },
        {
          label: 'Reference',
          items: [{ autogenerate: { directory: 'reference' } }],
        },
        {
          label: 'Concepts',
          items: [{ autogenerate: { directory: 'explanation' } }],
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
