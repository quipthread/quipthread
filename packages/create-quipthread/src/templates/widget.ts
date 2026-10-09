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
    '  data-quipthread',
    `  data-site-id="${config.siteId}"`,
    `  data-page-id="your-page-id"`,
    `  data-theme="${config.theme}"`,
    '></div>',
    `<script src="${config.apiBase}/embed.js" async></script>`,
  ].join('\n')
}

// Single-page apps mount through window.Quipthread (defined by embed.js) so
// the widget follows client-side navigation and is removed with the component.
const QUIPTHREAD_TYPES = `interface QuipthreadOptions {
  siteId: string
  pageId: string
  pageUrl?: string
  theme?: string
}

declare global {
  interface Window {
    Quipthread: {
      mount(el: Element, options: QuipthreadOptions): {
        update(options: Partial<QuipthreadOptions>): void
        unmount(): void
      }
    }
  }
}`

function loadScriptFunction(apiBase: string): string {
  return `// Loads embed.js once; it defines window.Quipthread as soon as it runs.
function loadQuipthread(): Promise<void> {
  if (window.Quipthread) return Promise.resolve()
  return new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = '${apiBase}/embed.js'
    script.onload = () => resolve()
    script.onerror = reject
    document.head.appendChild(script)
  })
}`
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
import { useEffect, useRef, useState } from 'react'

const API_BASE = '${config.apiBase}'
const SITE_ID = '${config.siteId}'

${QUIPTHREAD_TYPES}

interface Props {
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current pathname. */
  pageId?: string
}

export default function QuipthreadComments({ theme = '${config.theme}', pageId }: Props) {
  const pathname = usePathname()
  const resolvedPageId = pageId ?? pathname
  const ref = useRef<HTMLDivElement>(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    if (!ready || !ref.current) return
    const widget = window.Quipthread.mount(ref.current, {
      siteId: SITE_ID,
      pageId: resolvedPageId,
      pageUrl: window.location.href,
      theme,
    })
    return () => widget.unmount()
  }, [ready, resolvedPageId, theme])

  return (
    <>
      <div ref={ref} />
      <Script src={\`\${API_BASE}/embed.js\`} onReady={() => setReady(true)} />
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
    content: `import Script from 'next/script'
import { useRouter } from 'next/router'
import { useEffect, useRef, useState } from 'react'

const API_BASE = '${config.apiBase}'
const SITE_ID = '${config.siteId}'

${QUIPTHREAD_TYPES}

interface Props {
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current path without query or hash. */
  pageId?: string
}

export default function QuipthreadComments({ theme = '${config.theme}', pageId }: Props) {
  const { asPath } = useRouter()
  const resolvedPageId = pageId ?? asPath.split(/[?#]/)[0]
  const ref = useRef<HTMLDivElement>(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    if (!ready || !ref.current) return
    const widget = window.Quipthread.mount(ref.current, {
      siteId: SITE_ID,
      pageId: resolvedPageId,
      pageUrl: window.location.href,
      theme,
    })
    return () => widget.unmount()
  }, [ready, resolvedPageId, theme])

  return (
    <>
      <div ref={ref} />
      <Script src={\`\${API_BASE}/embed.js\`} onReady={() => setReady(true)} />
    </>
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
  data-quipthread
  data-site-id={SITE_ID}
  data-page-id={resolvedPageId}
  data-page-url={pageUrl}
  data-theme={theme}
></div>

<script is:inline src={\`\${API_BASE}/embed.js\`} async></script>
<!-- With view transitions, mount the widget on each new page. -->
<script is:inline>
  document.addEventListener('astro:page-load', () => window.Quipthread?.scan())
</script>
`,
  }
}

// ---------------------------------------------------------------------------
// Vue 3
// ---------------------------------------------------------------------------

function vueComponent(config: Config, routeImport: string): string {
  return `<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
${routeImport}
const SITE_ID = '${config.siteId}'

${QUIPTHREAD_TYPES}

${loadScriptFunction(config.apiBase)}

const props = withDefaults(defineProps<{
  theme?: 'auto' | 'light' | 'dark'
  /** Override the page ID. Defaults to the current route path. */
  pageId?: string
}>(), { theme: '${config.theme}' })

const el = ref<HTMLElement | null>(null)
const route = useRoute()
const pageId = () => props.pageId ?? route.path
let widget: ReturnType<Window['Quipthread']['mount']> | undefined
let unmounted = false

onMounted(async () => {
  await loadQuipthread()
  if (unmounted || !el.value) return
  widget = window.Quipthread.mount(el.value, {
    siteId: SITE_ID,
    pageId: pageId(),
    pageUrl: window.location.href,
    theme: props.theme,
  })
})
watch(pageId, (next) => widget?.update({ pageId: next, pageUrl: window.location.href }))
watch(() => props.theme, (theme) => widget?.update({ theme }))
onBeforeUnmount(() => {
  unmounted = true
  widget?.unmount()
})
</script>

<template>
  <div ref="el" />
</template>
`
}

function vueTemplate(config: Config): GeneratedFile {
  return {
    path: 'src/components/QuipthreadComments.vue',
    content: vueComponent(config, "import { useRoute } from 'vue-router'\n"),
  }
}

// ---------------------------------------------------------------------------
// Nuxt 3
// ---------------------------------------------------------------------------

function nuxtTemplates(config: Config): GeneratedFile[] {
  return [
    {
      // .client: the widget only runs in the browser.
      path: 'components/QuipthreadComments.client.vue',
      // Nuxt auto-imports useRoute.
      content: vueComponent(config, ''),
    },
  ]
}
