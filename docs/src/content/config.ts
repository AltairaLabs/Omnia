import { defineCollection, z } from 'astro:content';

// Schema for documentation pages
const docSchema = z.object({
  title: z.string(),
  description: z.string().optional(),
  order: z.number().optional(),
  draft: z.boolean().default(false),
});

// Define collections for each Diataxis category
export const collections = {
  'tutorials': defineCollection({
    type: 'content',
    schema: docSchema,
  }),
  'how-to': defineCollection({
    type: 'content',
    schema: docSchema,
  }),
  'reference': defineCollection({
    type: 'content',
    schema: docSchema,
  }),
  'explanation': defineCollection({
    type: 'content',
    schema: docSchema,
  }),
};
