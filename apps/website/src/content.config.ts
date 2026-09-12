import { defineCollection } from 'astro:content'
import { docsSchema } from '@astrojs/starlight/schema'
import { glob } from 'astro/loaders'

export const collections = {
  docs: defineCollection({
    loader: glob({
      pattern: 'docs/**/[^_]*.{markdown,mdown,mkdn,mkd,mdwn,md,mdx}',
      base: './src/content',
    }),
    schema: docsSchema(),
  }),
}
