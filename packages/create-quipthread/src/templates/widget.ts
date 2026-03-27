import type { Framework } from '../types.js'

export interface Config {
  framework: Framework
  apiBase: string
  siteId: string
  theme: 'auto' | 'light' | 'dark'
}

export interface GeneratedFile {
  path: string
  content: string
}

export function generateFiles(config: Config): GeneratedFile[] {
  switch (config.framework) {
    case 'nextjs-app':
      return [nextjsAppTemplate(config)]
    case 'nextjs-pages':
      return [nextjsPagesTemplate(config)]
    case 'astro':
      return [astroTemplate(config)]
    case 'vue':
      return [vueTemplate(config)]
    case 'nuxt':
      return nuxtTemplates(config)
    case 'vanilla':
      return []
  }
}

export function generateVanillaSnippet(config: Config): string {
  return [
    '<!-- Place this where you want comments to appear -->',
    '<div',
    `  id="comments"`,
    `  data-site-id="${config.siteId}"`,
    `  data-page-id="your-page-id"`,
    `  data-theme="${config.theme}"`,
    '></div>',
    `<script src="${config.apiBase}/embed.js" async></script>`,
  ].join('\n')
}

// ---------------------------------------------------------------------------
// Next.js — App Router
// ---------------------------------------------------------------------------

function nextjsAppTemplate(config: Config): GeneratedFile {
  return {
    path: 'components/QuipthreadComments.tsx',
    content: `'use client'

import Script from 'next/script'
import { usePathname } from 'next/navigation'

const API_BASE = '${config.apiBase}'
const SITE_ID = '${config.siteId}'

interface Props {
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current pathname. */
  pageId?: string
}

export default function QuipthreadComments({ theme = '${config.theme}', pageId }: Props) {
  const pathname = usePathname()

  return (
    <>
      <Script src={\`\${API_BASE}/embed.js\`} strategy="afterInteractive" />
      <div
        id="comments"
        data-site-id={SITE_ID}
        data-page-id={pageId ?? pathname}
        data-page-url={typeof window !== 'undefined' ? window.location.href : ''}
        data-theme={theme}
      />
    </>
  )
}
`,
  }
}

// ---------------------------------------------------------------------------
// Next.js — Pages Router
// ---------------------------------------------------------------------------

function nextjsPagesTemplate(config: Config): GeneratedFile {
  return {
    path: 'components/QuipthreadComments.tsx',
    content: `import { useEffect } from 'react'
import { useRouter } from 'next/router'

const API_BASE = '${config.apiBase}'
const SITE_ID = '${config.siteId}'

interface Props {
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current path. */
  pageId?: string
}

// Import this component with dynamic() to disable SSR:
//   import dynamic from 'next/dynamic'
//   const QuipthreadComments = dynamic(
//     () => import('@/components/QuipthreadComments'),
//     { ssr: false }
//   )
export default function QuipthreadComments({ theme = '${config.theme}', pageId }: Props) {
  const { asPath } = useRouter()

  useEffect(() => {
    if (document.getElementById('quipthread-script')) return
    const script = document.createElement('script')
    script.id = 'quipthread-script'
    script.src = \`\${API_BASE}/embed.js\`
    script.async = true
    document.body.appendChild(script)
  }, [])

  return (
    <div
      id="comments"
      data-site-id={SITE_ID}
      data-page-id={pageId ?? asPath}
      data-theme={theme}
    />
  )
}
`,
  }
}

// ---------------------------------------------------------------------------
// Astro
// ---------------------------------------------------------------------------

function astroTemplate(config: Config): GeneratedFile {
  return {
    path: 'src/components/QuipthreadComments.astro',
    content: `---
const API_BASE = '${config.apiBase}'
const SITE_ID = '${config.siteId}'

interface Props {
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current pathname. */
  pageId?: string
}

const { theme = '${config.theme}', pageId } = Astro.props
const resolvedPageId = pageId ?? Astro.url.pathname
const pageUrl = Astro.url.href
---

<div
  id="comments"
  data-site-id={SITE_ID}
  data-page-id={resolvedPageId}
  data-page-url={pageUrl}
  data-theme={theme}
></div>

<script is:inline define:vars={{ apiBase: API_BASE }}>
  ;(function () {
    if (document.getElementById('quipthread-script')) return
    var s = document.createElement('script')
    s.id = 'quipthread-script'
    s.src = apiBase + '/embed.js'
    s.async = true
    document.head.appendChild(s)
  })()
</script>
`,
  }
}

// ---------------------------------------------------------------------------
// Vue 3
// ---------------------------------------------------------------------------

function vueTemplate(config: Config): GeneratedFile {
  return {
    path: 'src/components/QuipthreadComments.vue',
    content: `<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'

const API_BASE = '${config.apiBase}'
const SITE_ID = '${config.siteId}'

withDefaults(defineProps<{
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current route path. */
  pageId?: string
}>(), { theme: '${config.theme}' })

const route = useRoute()

onMounted(() => {
  if (document.getElementById('quipthread-script')) return
  const script = document.createElement('script')
  script.id = 'quipthread-script'
  script.src = \`\${API_BASE}/embed.js\`
  script.async = true
  document.body.appendChild(script)
})
</script>

<template>
  <div
    id="comments"
    :data-site-id="SITE_ID"
    :data-page-id="pageId ?? route.path"
    :data-page-url="route.fullPath"
    :data-theme="theme"
  />
</template>
`,
  }
}

// ---------------------------------------------------------------------------
// Nuxt 3
// ---------------------------------------------------------------------------

function nuxtTemplates(config: Config): GeneratedFile[] {
  return [
    {
      path: 'plugins/quipthread.client.ts',
      content: `const API_BASE = '${config.apiBase}'

// Loads the Quipthread embed script once on the client.
// The QuipthreadComments component renders the mount point.
export default defineNuxtPlugin(() => {
  if (document.getElementById('quipthread-script')) return
  const script = document.createElement('script')
  script.id = 'quipthread-script'
  script.src = \`\${API_BASE}/embed.js\`
  script.async = true
  document.head.appendChild(script)
})
`,
    },
    {
      path: 'components/QuipthreadComments.vue',
      content: `<script setup lang="ts">
const SITE_ID = '${config.siteId}'

withDefaults(defineProps<{
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current route path. */
  pageId?: string
}>(), { theme: '${config.theme}' })

const route = useRoute()
</script>

<template>
  <div
    id="comments"
    :data-site-id="SITE_ID"
    :data-page-id="pageId ?? route.path"
    :data-page-url="route.fullPath"
    :data-theme="theme"
  />
</template>
`,
    },
  ]
}
