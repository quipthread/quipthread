import sitemap from '@astrojs/sitemap'
import starlight from '@astrojs/starlight'
import { defineConfig } from 'astro/config'

const backendUrl = process.env.PUBLIC_BACKEND_URL ?? 'http://localhost:8080'

export default defineConfig({
  site: 'https://quipthread.com',
  vite: {
    server: {
      proxy: {
        '/auth': { target: backendUrl, changeOrigin: true },
        '/api': { target: backendUrl, changeOrigin: true },
      },
    },
  },
  integrations: [
    sitemap(),
    starlight({
      expressiveCode: {
        defaultProps: {
          // Suppress the empty frame header bar on code blocks that have no
          // title — by default expressive-code renders a header regardless,
          // producing a dark strip above the code content.
          frame: 'none',
        },
      },
      title: 'Quipthread Documentation',
      components: {
        SiteTitle: './src/components/SiteTitle.astro',
        SocialIcons: './src/components/SocialIcons.astro',
      },
      head: [
        {
          tag: 'link',
          attrs: { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' },
        },
        {
          tag: 'link',
          attrs: { rel: 'manifest', href: '/manifest.webmanifest' },
        },
        {
          tag: 'meta',
          attrs: { name: 'theme-color', content: '#1A1A1A' },
        },
        {
          tag: 'link',
          attrs: { rel: 'apple-touch-icon', href: '/icon-192.png' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:image', content: 'https://quipthread.com/og-default.png' },
        },
        {
          tag: 'meta',
          attrs: { name: 'twitter:card', content: 'summary_large_image' },
        },
        {
          tag: 'link',
          attrs: { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        },
        {
          tag: 'link',
          attrs: { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'stylesheet',
            href: 'https://fonts.googleapis.com/css2?family=Syne:wght@700;800&display=swap',
          },
        },
      ],
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/quipthread/quipthread',
        },
      ],
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Overview', slug: 'docs' },
            { label: 'Cloud (Managed)', slug: 'docs/cloud/getting-started' },
            { label: 'Self-Hosting', slug: 'docs/guides/self-hosting' },
          ],
        },
        {
          label: 'Self-Hosting Guides',
          items: [
            { label: 'OAuth Setup', slug: 'docs/guides/oauth-setup' },
            { label: 'Email Auth', slug: 'docs/guides/email-auth' },
            { label: 'Notifications', slug: 'docs/guides/notifications' },
            { label: 'Docker', slug: 'docs/guides/docker' },
          ],
        },
        {
          label: 'Embed Widget',
          items: [
            { label: 'Reference', slug: 'docs/embed/reference' },
            { label: 'Theming', slug: 'docs/embed/theming' },
          ],
        },
        {
          label: 'Dashboard',
          items: [
            { label: 'Moderation', slug: 'docs/dashboard/moderation' },
            { label: 'Import & Export', slug: 'docs/dashboard/import-export' },
            { label: 'Security & Spam Protection', slug: 'docs/dashboard/security' },
          ],
        },
        {
          label: 'Reference',
          items: [{ label: 'Environment Variables', slug: 'docs/reference/environment-variables' }],
        },
      ],
    }),
  ],
})
