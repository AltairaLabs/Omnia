// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightThemeGalaxy from 'starlight-theme-galaxy';

// https://astro.build/config
export default defineConfig({
  site: 'https://omnia.altairalabs.ai',
  integrations: [
    starlight({
      plugins: [starlightThemeGalaxy()],
      title: 'Omnia',
      description: 'Kubernetes operator for managing AI agent deployments',
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
        replacesTitle: false,
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/altairalabs/omnia' },
      ],
      customCss: [
        './src/styles/custom.css',
      ],
      // Custom components for version switching
      components: {
        Header: './src/components/Header.astro',
        PageTitle: './src/components/PageTitle.astro',
      },
      sidebar: [
        {
          label: 'Tutorials',
          autogenerate: { directory: 'tutorials' },
        },
        {
          label: 'How-To Guides',
          autogenerate: { directory: 'how-to' },
        },
        {
          label: 'Reference',
          autogenerate: { directory: 'reference' },
        },
        {
          label: 'Concepts',
          autogenerate: { directory: 'explanation' },
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/altairalabs/omnia/edit/main/docs/',
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
