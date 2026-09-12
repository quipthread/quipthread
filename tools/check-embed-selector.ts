import { resolve } from 'node:path'

const EXPECTED_SELECTOR = 'comments'
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

async function readSource(path: string): Promise<string | undefined> {
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
  return source
}

function requireText(path: string, source: string, expected: string, description: string): void {
  if (!source.includes(expected)) {
    violation(path, `missing ${description}: ${expected}`)
  }
}

function checkSelectorMarkup(path: string, source: string): void {
  requireText(path, source, `id="${EXPECTED_SELECTOR}"`, 'canonical mount id')
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
    const branch = source.slice(start, end === -1 ? source.length : end)
    requireText(path, branch, `id="${EXPECTED_SELECTOR}"`, `${framework} canonical mount id`)
  }
}

function checkTemplateFunctions(path: string, source: string): void {
  const functions = [
    'generateVanillaSnippet',
    'nextjsAppTemplate',
    'nextjsPagesTemplate',
    'astroTemplate',
    'vueTemplate',
    'nuxtTemplates',
  ]

  for (const [index, name] of functions.entries()) {
    const marker = `function ${name}(`
    const start = source.indexOf(marker)
    if (start === -1) {
      violation(path, `missing template function: ${name}`)
      continue
    }

    const nextStart = functions
      .slice(index + 1)
      .map((nextName) => source.indexOf(`function ${nextName}(`, start + marker.length))
      .filter((position) => position !== -1)
      .sort((a, b) => a - b)[0]
    const template = source.slice(start, nextStart ?? source.length)
    requireText(path, template, `id="${EXPECTED_SELECTOR}"`, `${name} canonical mount id`)
  }
}

async function main(): Promise<void> {
  const runtimePath = 'embed/src/main.tsx'
  const runtime = await readSource(runtimePath)
  if (runtime) {
    requireText(
      runtimePath,
      runtime,
      `export const EMBED_MOUNT_ID = '${EXPECTED_SELECTOR}'`,
      'exported mount id',
    )
    requireText(
      runtimePath,
      runtime,
      'document.getElementById(EMBED_MOUNT_ID)',
      'runtime mount lookup',
    )
    requireText(
      runtimePath,
      runtime,
      ['#', '$', '{EMBED_MOUNT_ID}'].join(''),
      'runtime mount id in error',
    )
  }

  const harnessPath = 'embed/index.html'
  const harness = await readSource(harnessPath)
  if (harness) checkSelectorMarkup(harnessPath, harness)

  const dashboardPath = 'apps/dashboard/src/components/EmbedCodeGenerator.tsx'
  const dashboard = await readSource(dashboardPath)
  if (dashboard) checkDashboardBranches(dashboardPath, dashboard)

  for (const path of [
    'packages/create-quipthread/src/templates.ts',
    'packages/create-quipthread/src/templates/widget.ts',
  ]) {
    const source = await readSource(path)
    if (source) checkTemplateFunctions(path, source)
  }

  for (const path of [
    'apps/website/src/content/docs/cloud/getting-started.mdx',
    'apps/website/src/content/docs/embed/reference.mdx',
    'apps/website/src/content/docs/embed/theming.mdx',
    'apps/website/src/content/docs/guides/self-hosting.mdx',
  ]) {
    const source = await readSource(path)
    if (source) checkSelectorMarkup(path, source)
  }

  if (violations.length > 0) {
    for (const { path, message } of violations) {
      console.error(`[embed-selector] ${path}: ${message}`)
    }
    process.exitCode = 1
    return
  }

  console.log(`[embed-selector] ${EXPECTED_SELECTOR} contract verified`)
}

await main()
