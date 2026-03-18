import { existsSync, readdirSync } from 'fs'
import { join } from 'path'
import type { Framework } from './index.js'

export function detectFramework(cwd: string): Framework | null {
  const has = (rel: string) => existsSync(join(cwd, rel))

  // Nuxt must be checked before Vue — Nuxt projects also have vite.config files
  if (has('nuxt.config.js') || has('nuxt.config.ts') || has('nuxt.config.mjs')) {
    return 'nuxt'
  }

  if (
    has('next.config.js') || has('next.config.ts') ||
    has('next.config.mjs') || has('next.config.cjs')
  ) {
    // Prefer Pages Router only when there's a pages/ dir but no app/ dir
    const hasAppDir = has('app') || has('src/app')
    const hasPagesDir = has('pages') || has('src/pages')
    if (hasPagesDir && !hasAppDir) return 'nextjs-pages'
    return 'nextjs-app'
  }

  if (
    has('astro.config.js') || has('astro.config.mjs') ||
    has('astro.config.ts') || has('astro.config.cjs')
  ) {
    return 'astro'
  }

  // Vue: Vite config + at least one .vue file in src/
  if (has('vite.config.js') || has('vite.config.ts') || has('vite.config.mjs')) {
    try {
      const srcDir = join(cwd, 'src')
      if (existsSync(srcDir) && readdirSync(srcDir).some(f => f.endsWith('.vue'))) {
        return 'vue'
      }
    } catch {
      // ignore unreadable src/
    }
  }

  return null
}
