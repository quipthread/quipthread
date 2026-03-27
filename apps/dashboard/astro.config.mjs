import preact from '@astrojs/preact'
import { defineConfig } from 'astro/config'

export default defineConfig({
  integrations: [preact({ compat: true })],
  output: 'static',
})
