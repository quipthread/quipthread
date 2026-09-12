import { randomBytes } from 'node:crypto'
import { existsSync } from 'node:fs'
import { mkdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { cancel, isCancel, log, multiselect, note, password, text } from '@clack/prompts'
import {
  caddyfile,
  dockerCompose,
  dockerfile,
  dotEnv,
  gitignore,
  type ProjectConfig,
} from '../templates/project.js'

import { promptSmtp } from './smtp.js'

export async function newProjectFlow(): Promise<void> {
  const rawName = await text({
    message: 'Project directory name',
    placeholder: 'my-quipthread',
    defaultValue: 'my-quipthread',
    validate: (v) => {
      if (existsSync(join(process.cwd(), v.trim())))
        return 'Choose a new directory; this one already exists'
      if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]*$/.test(v.trim()))
        return 'Use letters, numbers, hyphens, or underscores; start with a letter or number'
    },
  })
  if (isCancel(rawName)) {
    cancel('Cancelled.')
    process.exit(0)
  }
  const projectName = rawName.trim()

  const rawBaseUrl = await text({
    message: 'Base URL (where Quipthread will be hosted)',
    placeholder: 'https://comments.example.com',
    validate: (v) => {
      if (!v.trim()) return 'Base URL is required'
      try {
        const url = new URL(v)
        if (
          !['http:', 'https:'].includes(url.protocol) ||
          url.username ||
          url.password ||
          (url.pathname !== '/' && url.pathname !== '') ||
          url.search ||
          url.hash
        ) {
          return 'Enter an HTTP or HTTPS origin without a path, credentials, or query'
        }
        if (url.protocol === 'https:' && url.port) return 'Use the default HTTPS port (443)'
        if (url.protocol === 'http:' && !['localhost', '127.0.0.1'].includes(url.hostname)) {
          return 'Use HTTPS for a public server, or HTTP with localhost for local testing'
        }
      } catch {
        return 'Enter a valid URL (e.g. https://comments.example.com)'
      }
    },
  })
  if (isCancel(rawBaseUrl)) {
    cancel('Cancelled.')
    process.exit(0)
  }
  const baseUrl = rawBaseUrl.trim().replace(/\/$/, '')

  const publisher = await text({
    message: 'Website origins that will embed comments (comma-separated)',
    placeholder: 'https://example.com,https://www.example.com',
    validate: (value) => {
      try {
        for (const entry of value.split(',')) {
          const url = new URL(entry.trim())
          if (
            !['http:', 'https:'].includes(url.protocol) ||
            url.username ||
            url.password ||
            url.pathname !== '/' ||
            url.search ||
            url.hash ||
            url.hostname.includes('*')
          ) {
            return 'Enter HTTP or HTTPS origins without paths or wildcards'
          }
        }
      } catch {
        return 'Enter at least one valid website origin'
      }
    },
  })
  if (isCancel(publisher)) {
    cancel('Cancelled.')
    process.exit(0)
  }
  const allowedOrigins = publisher
    .split(',')
    .map((origin) => new URL(origin.trim()).origin)
    .join(',')

  const providers = await multiselect({
    message: 'Which auth providers do you want to enable?',
    options: [
      { value: 'github', label: 'GitHub OAuth' },
      { value: 'google', label: 'Google OAuth' },
      { value: 'email', label: 'Email / Password' },
    ],
    required: true,
  })
  if (isCancel(providers)) {
    cancel('Cancelled.')
    process.exit(0)
  }
  const enabledProviders = providers

  let githubClientId = ''
  let githubClientSecret = ''
  if (enabledProviders.includes('github')) {
    const id = await text({
      message: 'GitHub OAuth Client ID',
      validate: (v) => {
        if (!v.trim()) return 'Required'
      },
    })
    if (isCancel(id)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    githubClientId = id.trim()

    const secret = await password({
      message: 'GitHub OAuth Client Secret',
      validate: (v) => {
        if (!v.trim()) return 'Required'
      },
    })
    if (isCancel(secret)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    githubClientSecret = secret.trim()
  }

  let googleClientId = ''
  let googleClientSecret = ''
  if (enabledProviders.includes('google')) {
    const id = await text({
      message: 'Google OAuth Client ID',
      validate: (v) => {
        if (!v.trim()) return 'Required'
      },
    })
    if (isCancel(id)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    googleClientId = id.trim()

    const secret = await password({
      message: 'Google OAuth Client Secret',
      validate: (v) => {
        if (!v.trim()) return 'Required'
      },
    })
    if (isCancel(secret)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    googleClientSecret = secret.trim()
  }

  const emailAuthEnabled = enabledProviders.includes('email')

  const smtp = await promptSmtp(emailAuthEnabled)

  const jwtSecret = randomBytes(32).toString('hex')

  const cfg: ProjectConfig = {
    baseUrl,
    allowedOrigins,
    jwtSecret,
    githubClientId,
    githubClientSecret,
    googleClientId,
    googleClientSecret,
    emailAuthEnabled,
    ...smtp,
  }

  const dir = join(process.cwd(), projectName)
  await mkdir(dir)
  await mkdir(join(dir, 'data'))

  await writeFile(join(dir, 'docker-compose.yml'), dockerCompose(baseUrl), 'utf8')
  log.success('Created docker-compose.yml')

  await writeFile(join(dir, '.env'), dotEnv(cfg), { encoding: 'utf8', mode: 0o600 })
  log.success('Created .env')

  await writeFile(join(dir, '.gitignore'), gitignore(), 'utf8')
  log.success('Created .gitignore')

  await writeFile(join(dir, 'Dockerfile'), dockerfile(), 'utf8')
  await writeFile(join(dir, '.dockerignore'), '.env\ndata/\n.git/\n', 'utf8')
  if (new URL(baseUrl).protocol === 'https:') {
    await writeFile(join(dir, 'Caddyfile'), caddyfile(baseUrl), 'utf8')
  }

  await writeFile(join(dir, 'data', '.gitkeep'), '', 'utf8')

  note(
    [
      `cd ${projectName}`,
      '',
      'For HTTPS, point DNS at this server and open TCP ports 80 and 443.',
      'Register OAuth callbacks under BASE_URL/auth/github/callback or /auth/google/callback.',
      '',
      'Start the published Quipthread release:',
      '  docker compose pull',
      '  docker compose up -d',
      '',
      `Open ${baseUrl}/login and create the first admin account.`,
      'The image follows published releases; it does not track repository main.',
      'For a source build: https://quipthread.com/docs/guides/docker/',
      '',
      'Keep .env out of version control — it contains your JWT secret.',
    ].join('\n'),
    'Next steps',
  )
}
