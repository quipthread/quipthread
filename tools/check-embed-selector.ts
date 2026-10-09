import { resolve } from 'node:path'

// The widget mounts into [data-quipthread] elements (or through
// window.Quipthread.mount). The original id="comments" markup is only claimed
// when it carries data-site-id, so new snippets and docs must not use it: a
// theme's own #comments (WordPress) would collide with it.
const MOUNT_ATTRIBUTE = 'data-quipthread'
const MOUNT_API = 'Quipthread.mount('
const RUNTIME_SELECTOR = `'[data-quipthread], [id="comments"][data-site-id]'`
const FORBIDDEN_SELECTOR = 'quipthread-comments'
const ROOT = resolve(import.meta.dir, '..')

type Violation = {
  path: string
  message: string
}

const violations: Violation[] = []

function violation(path: string, message: string): void {
  violations.push({ path, message })
}

async function readSource(path: string, allowLegacy = false): Promise<string | undefined> {
  const absolutePath = resolve(ROOT, path)
  const file = Bun.file(absolutePath)

  if (!(await file.exists())) {
    violation(path, 'file does not exist')
    return undefined
  }

  const source = await file.text()
  if (source.includes(FORBIDDEN_SELECTOR)) {
    violation(path, `contains forbidden selector "${FORBIDDEN_SELECTOR}"`)
  }
  // Prose may mention the legacy markup in `code`; markup must not use it.
  for (const [index, line] of allowLegacy ? [] : source.split('\n').entries()) {
    if (line.includes('id="comments"') && !line.includes('`')) {
      violation(path, `line ${index + 1} uses the legacy id="comments" mount point`)
    }
  }
  return source
}

function requireText(path: string, source: string, expected: string, description: string): void {
  if (!source.includes(expected)) {
    violation(path, `missing ${description}: ${expected}`)
  }
}

function requireMount(path: string, source: string, description: string): void {
  if (!source.includes(MOUNT_ATTRIBUTE) && !source.includes(MOUNT_API)) {
    violation(
      path,
      `${description} neither marks a ${MOUNT_ATTRIBUTE} container nor calls ${MOUNT_API}`,
    )
  }
}

function checkDashboardBranches(path: string, source: string): void {
  const frameworks = ['next-app', 'next-pages', 'astro', 'vue', 'nuxt', 'vanilla']

  for (const [index, framework] of frameworks.entries()) {
    const marker = `case '${framework}':`
    const start = source.indexOf(marker)
    if (start === -1) {
      violation(path, `missing dashboard generator branch: ${framework}`)
      continue
    }

    const nextMarker = frameworks[index + 1] ? `\n    case '${frameworks[index + 1]}':` : undefined
    const end = nextMarker ? source.indexOf(nextMarker, start + marker.length) : -1
    requireMount(
      path,
      source.slice(start, end === -1 ? source.length : end),
      `${framework} snippet`,
    )
  }
}

async function main(): Promise<void> {
  const runtimePath = 'embed/src/mount.tsx'
  // The runtime is the one place that still claims the legacy markup.
  const runtime = await readSource(runtimePath, true)
  if (runtime) {
    requireText(
      runtimePath,
      runtime,
      `export const AUTO_MOUNT_SELECTOR = ${RUNTIME_SELECTOR}`,
      'auto-mount selector',
    )
  }

  const harnessPath = 'embed/index.html'
  const harness = await readSource(harnessPath)
  if (harness) requireText(harnessPath, harness, MOUNT_ATTRIBUTE, 'mount attribute')

  const dashboardPath = 'apps/dashboard/src/components/EmbedCodeGenerator.tsx'
  const dashboard = await readSource(dashboardPath)
  if (dashboard) checkDashboardBranches(dashboardPath, dashboard)

  const templatesPath = 'packages/create-quipthread/src/templates/widget.ts'
  const templates = await readSource(templatesPath)
  if (templates) {
    requireText(templatesPath, templates, MOUNT_ATTRIBUTE, 'mount attribute (vanilla, Astro)')
    requireText(templatesPath, templates, MOUNT_API, 'mount API (Next.js, Vue, Nuxt)')
  }

  for (const path of [
    'apps/website/src/content/docs/cloud/getting-started.mdx',
    'apps/website/src/content/docs/embed/reference.mdx',
    'apps/website/src/content/docs/embed/theming.mdx',
    'apps/website/src/content/docs/guides/self-hosting.mdx',
  ]) {
    const source = await readSource(path)
    if (source) requireText(path, source, MOUNT_ATTRIBUTE, 'mount attribute')
  }

  for (const path of ['backend/handlers/embed_preview.go', 'backend/main.go']) {
    const source = await readSource(path)
    if (source) requireText(path, source, MOUNT_ATTRIBUTE, 'mount attribute')
  }

  if (violations.length > 0) {
    for (const { path, message } of violations) {
      console.error(`[embed-selector] ${path}: ${message}`)
    }
    process.exitCode = 1
    return
  }

  console.log(`[embed-selector] ${MOUNT_ATTRIBUTE} contract verified`)
}

await main()
