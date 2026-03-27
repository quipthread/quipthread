import { mkdir, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { cancel, isCancel, log, note, select, text } from '@clack/prompts'
import { detectFramework } from '../detect.js'
import { type Config, generateFiles, generateVanillaSnippet } from '../templates/widget.js'
import type { Framework } from '../types.js'

export async function addCommentsFlow(): Promise<void> {
  const detected = detectFramework(process.cwd())

  const framework = await select({
    message: 'Which framework are you using?',
    initialValue: (detected ?? 'vanilla') as Framework,
    options: [
      { value: 'nextjs-app' as const, label: 'Next.js', hint: 'App Router' },
      { value: 'nextjs-pages' as const, label: 'Next.js', hint: 'Pages Router' },
      { value: 'astro' as const, label: 'Astro' },
      { value: 'vue' as const, label: 'Vue 3' },
      { value: 'nuxt' as const, label: 'Nuxt 3' },
      { value: 'vanilla' as const, label: 'Vanilla HTML', hint: 'prints snippet only' },
    ],
  })

  if (isCancel(framework)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  const rawApiBase = await text({
    message: 'Quipthread API base URL',
    placeholder: 'https://comments.example.com',
    validate: (v) => {
      if (!v.trim()) return 'API base URL is required'
      try {
        new URL(v)
      } catch {
        return 'Enter a valid URL (e.g. https://comments.example.com)'
      }
    },
  })

  if (isCancel(rawApiBase)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  const rawSiteId = await text({
    message: 'Site ID  (from your Quipthread dashboard → Sites)',
    placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx',
    validate: (v) => {
      if (!v.trim()) return 'Site ID is required'
    },
  })

  if (isCancel(rawSiteId)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  const theme = await select({
    message: 'Default theme',
    initialValue: 'auto' as const,
    options: [
      { value: 'auto' as const, label: 'Auto', hint: 'follows system preference' },
      { value: 'light' as const, label: 'Light' },
      { value: 'dark' as const, label: 'Dark' },
    ],
  })

  if (isCancel(theme)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  const config: Config = {
    framework: framework as Framework,
    apiBase: (rawApiBase as string).trim().replace(/\/$/, ''),
    siteId: (rawSiteId as string).trim(),
    theme: theme as 'auto' | 'light' | 'dark',
  }

  if (config.framework === 'vanilla') {
    note(generateVanillaSnippet(config), 'Add this to your HTML')
    return
  }

  const files = generateFiles(config)

  for (const { path: filePath, content } of files) {
    const fullPath = join(process.cwd(), filePath)
    await mkdir(dirname(fullPath), { recursive: true })
    await writeFile(fullPath, content, 'utf8')
    log.success(`Created ${filePath}`)
  }

  note(usageInstructions(config.framework), 'Next steps')
}

function usageInstructions(framework: Framework): string {
  switch (framework) {
    case 'nextjs-app':
      return [
        'Import the component in your page:',
        '',
        "  import QuipthreadComments from '@/components/QuipthreadComments'",
        '',
        '  export default function Page() {',
        '    return <QuipthreadComments />',
        '  }',
      ].join('\n')

    case 'nextjs-pages':
      return [
        'Import with ssr: false to disable server-side rendering:',
        '',
        "  import dynamic from 'next/dynamic'",
        '  const QuipthreadComments = dynamic(',
        "    () => import('@/components/QuipthreadComments'),",
        '    { ssr: false }',
        '  )',
        '',
        '  export default function Page() {',
        '    return <QuipthreadComments />',
        '  }',
      ].join('\n')

    case 'astro':
      return [
        'Import the component in your Astro pages or layouts:',
        '',
        '  ---',
        "  import QuipthreadComments from '../components/QuipthreadComments.astro'",
        '  ---',
        '',
        '  <QuipthreadComments />',
      ].join('\n')

    case 'vue':
      return [
        'Import the component in your Vue pages:',
        '',
        '  <script setup>',
        "  import QuipthreadComments from '@/components/QuipthreadComments.vue'",
        '  </script>',
        '',
        '  <template>',
        '    <QuipthreadComments />',
        '  </template>',
      ].join('\n')

    case 'nuxt':
      return [
        'The plugin loads the embed script automatically.',
        'Nuxt auto-imports components, so just use it directly:',
        '',
        '  <template>',
        '    <QuipthreadComments />',
        '  </template>',
      ].join('\n')

    default:
      return ''
  }
}
