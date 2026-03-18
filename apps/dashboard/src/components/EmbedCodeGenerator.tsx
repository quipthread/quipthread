import { useState } from 'preact/hooks'

interface Props {
  siteId: string
  apiBase: string
}

type Framework = 'next-app' | 'next-pages' | 'astro' | 'vue' | 'nuxt' | 'vanilla'

const TABS: { id: Framework; label: string }[] = [
  { id: 'next-app', label: 'Next.js App' },
  { id: 'next-pages', label: 'Next.js Pages' },
  { id: 'astro', label: 'Astro' },
  { id: 'vue', label: 'Vue' },
  { id: 'nuxt', label: 'Nuxt' },
  { id: 'vanilla', label: 'HTML' },
]

function getSnippet(framework: Framework, siteId: string, apiBase: string): string {
  switch (framework) {
    case 'next-app':
      return `'use client'

import Script from 'next/script'
import { usePathname } from 'next/navigation'

export function Comments() {
  const pathname = usePathname()
  return (
    <>
      <div
        id="quipthread-comments"
        data-site-id="${siteId}"
        data-page-id={pathname}
      />
      <Script
        src="${apiBase}/embed.js"
        strategy="afterInteractive"
      />
    </>
  )
}`

    case 'next-pages':
      return `import { useEffect } from 'react'
import { useRouter } from 'next/router'

// Wrap with: dynamic(() => import('./Comments'), { ssr: false })
export function Comments() {
  const { asPath } = useRouter()
  useEffect(() => {
    const s = document.createElement('script')
    s.src = '${apiBase}/embed.js'
    s.async = true
    document.body.appendChild(s)
    return () => s.remove()
  }, [])

  return (
    <div
      id="quipthread-comments"
      data-site-id="${siteId}"
      data-page-id={asPath}
    />
  )
}`

    case 'astro':
      return `---
const pageId = Astro.url.pathname
---

<div
  id="quipthread-comments"
  data-site-id="${siteId}"
  data-page-id={pageId}
/>

<script define:vars={{ apiBase: '${apiBase}' }} is:inline>
  const s = document.createElement('script')
  s.src = apiBase + '/embed.js'
  s.async = true
  document.head.appendChild(s)
</script>`

    case 'vue':
      return `<template>
  <div
    id="quipthread-comments"
    data-site-id="${siteId}"
    :data-page-id="$route.path"
  />
</template>

<script setup>
import { onMounted } from 'vue'

onMounted(() => {
  const s = document.createElement('script')
  s.src = '${apiBase}/embed.js'
  s.async = true
  document.body.appendChild(s)
})
</script>`

    case 'nuxt':
      return `// plugins/quipthread.client.ts
export default defineNuxtPlugin(() => {
  const s = document.createElement('script')
  s.src = '${apiBase}/embed.js'
  s.async = true
  document.body.appendChild(s)
})

// components/QuipthreadComments.vue
// <template>
//   <div
//     id="quipthread-comments"
//     data-site-id="${siteId}"
//     :data-page-id="useRoute().path"
//   />
// </template>`

    case 'vanilla':
      return `<div
  id="quipthread-comments"
  data-site-id="${siteId}"
  data-page-id="/your-page-path"
></div>

<script src="${apiBase}/embed.js" async></script>`
  }
}

export default function EmbedCodeGenerator({ siteId, apiBase }: Props) {
  const [tab, setTab] = useState<Framework>('next-app')
  const [copied, setCopied] = useState(false)

  const snippet = getSnippet(tab, siteId, apiBase)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(snippet)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard unavailable (e.g. non-HTTPS) — fallback silently
    }
  }

  return (
    <div>
      {/* Framework tabs */}
      <div
        style={{
          display: 'flex',
          gap: 2,
          borderBottom: '1px solid var(--border)',
          marginBottom: 0,
        }}
      >
        {TABS.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            style={{
              background: 'none',
              border: 'none',
              borderBottom: `2px solid ${tab === t.id ? 'var(--amber)' : 'transparent'}`,
              marginBottom: -1,
              padding: '0.5rem 0.875rem',
              cursor: 'pointer',
              fontFamily: 'var(--f-ui)',
              fontSize: '0.8125rem',
              fontWeight: 500,
              color: tab === t.id ? 'var(--text)' : 'var(--muted)',
              transition: 'color 0.12s',
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Code block */}
      <div
        style={{
          position: 'relative',
          background: 'var(--ink)',
          borderRadius: '0 0 8px 8px',
          border: '1px solid var(--border)',
          borderTop: 'none',
        }}
      >
        <button
          onClick={copy}
          style={{
            position: 'absolute',
            top: '0.625rem',
            right: '0.625rem',
            background: 'rgba(255,255,255,0.08)',
            border: '1px solid rgba(255,255,255,0.12)',
            borderRadius: 5,
            color: copied ? '#86efac' : 'rgba(247, 244, 239, 0.65)',
            padding: '0.25rem 0.625rem',
            cursor: 'pointer',
            fontFamily: 'var(--f-ui)',
            fontSize: '0.75rem',
            fontWeight: 500,
            transition: 'color 0.15s, background 0.15s',
          }}
        >
          {copied ? 'Copied' : 'Copy'}
        </button>

        <pre
          style={{
            margin: 0,
            padding: '1.25rem 1.25rem',
            overflowX: 'auto',
            fontSize: '0.8125rem',
            lineHeight: 1.65,
            color: 'rgba(247, 244, 239, 0.88)',
            fontFamily: 'var(--f-mono)',
            whiteSpace: 'pre',
          }}
        >
          <code>{snippet}</code>
        </pre>
      </div>

      {/* CLI hint */}
      <p
        style={{
          marginTop: '1rem',
          fontSize: '0.8125rem',
          color: 'var(--muted)',
        }}
      >
        Alternatively, run{' '}
        <code
          style={{
            fontFamily: 'var(--f-mono)',
            fontSize: '0.8125rem',
            background: 'var(--surface)',
            padding: '0.1em 0.4em',
            borderRadius: 3,
            color: 'var(--text)',
          }}
        >
          bunx create-quipthread
        </code>{' '}
        in your project directory to scaffold the integration automatically.
      </p>
    </div>
  )
}
