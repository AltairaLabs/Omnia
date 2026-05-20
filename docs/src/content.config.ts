// Astro v6 dropped the legacy content-collections backward-compat that
// let pre-Content-Layer projects build without a loader. Starlight
// v0.30+ ships its own `docsLoader()` we must wire in explicitly here,
// otherwise the `docs` collection resolves to zero entries and the
// build emits a single index page. See Starlight CHANGELOG 0.30 →
// "Update your collections".
import { defineCollection } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';

export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
};
