import { text, password, multiselect, confirm, isCancel, cancel, log, note } from '@clack/prompts'
import { writeFile, mkdir } from 'fs/promises'
import { join } from 'path'
import { randomBytes } from 'crypto'
import { dockerCompose, dotEnv, gitignore, type ProjectConfig } from '../templates/project.js'

export async function newProjectFlow(): Promise<void> {
  const rawName = await text({
    message: 'Project directory name',
    placeholder: 'my-quipthread',
    defaultValue: 'my-quipthread',
    validate: (v) => { if (!v.trim()) return 'Directory name is required' },
  })
  if (isCancel(rawName)) { cancel('Cancelled.'); process.exit(0) }
  const projectName = (rawName as string).trim()

  const rawBaseUrl = await text({
    message: 'Base URL (where Quipthread will be hosted)',
    placeholder: 'https://comments.example.com',
    validate: (v) => {
      if (!v.trim()) return 'Base URL is required'
      try { new URL(v) } catch { return 'Enter a valid URL (e.g. https://comments.example.com)' }
    },
  })
  if (isCancel(rawBaseUrl)) { cancel('Cancelled.'); process.exit(0) }
  const baseUrl = (rawBaseUrl as string).trim().replace(/\/$/, '')

  const providers = await multiselect({
    message: 'Which auth providers do you want to enable?',
    options: [
      { value: 'github', label: 'GitHub OAuth' },
      { value: 'google', label: 'Google OAuth' },
      { value: 'email', label: 'Email / Password' },
    ],
    required: true,
  })
  if (isCancel(providers)) { cancel('Cancelled.'); process.exit(0) }
  const enabledProviders = providers as string[]

  let githubClientId = ''
  let githubClientSecret = ''
  if (enabledProviders.includes('github')) {
    const id = await text({
      message: 'GitHub OAuth Client ID',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(id)) { cancel('Cancelled.'); process.exit(0) }
    githubClientId = (id as string).trim()

    const secret = await password({
      message: 'GitHub OAuth Client Secret',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(secret)) { cancel('Cancelled.'); process.exit(0) }
    githubClientSecret = (secret as string).trim()
  }

  let googleClientId = ''
  let googleClientSecret = ''
  if (enabledProviders.includes('google')) {
    const id = await text({
      message: 'Google OAuth Client ID',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(id)) { cancel('Cancelled.'); process.exit(0) }
    googleClientId = (id as string).trim()

    const secret = await password({
      message: 'Google OAuth Client Secret',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(secret)) { cancel('Cancelled.'); process.exit(0) }
    googleClientSecret = (secret as string).trim()
  }

  const emailAuthEnabled = enabledProviders.includes('email')

  let smtpHost = ''
  let smtpPort = '587'
  let smtpUser = ''
  let smtpPass = ''
  let smtpFrom = ''

  const wantSmtp = await confirm({
    message: 'Configure SMTP for email notifications and verification?',
    initialValue: emailAuthEnabled,
  })
  if (isCancel(wantSmtp)) { cancel('Cancelled.'); process.exit(0) }

  if (wantSmtp) {
    const host = await text({
      message: 'SMTP host',
      placeholder: 'smtp.example.com',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(host)) { cancel('Cancelled.'); process.exit(0) }
    smtpHost = (host as string).trim()

    const port = await text({
      message: 'SMTP port',
      placeholder: '587',
      defaultValue: '587',
    })
    if (isCancel(port)) { cancel('Cancelled.'); process.exit(0) }
    smtpPort = (port as string).trim()

    const user = await text({
      message: 'SMTP username',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(user)) { cancel('Cancelled.'); process.exit(0) }
    smtpUser = (user as string).trim()

    const pass = await password({
      message: 'SMTP password',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(pass)) { cancel('Cancelled.'); process.exit(0) }
    smtpPass = (pass as string).trim()

    const from = await text({
      message: 'From address (e.g. Quipthread <noreply@example.com>)',
      validate: (v) => { if (!v.trim()) return 'Required' },
    })
    if (isCancel(from)) { cancel('Cancelled.'); process.exit(0) }
    smtpFrom = (from as string).trim()
  }

  const jwtSecret = randomBytes(32).toString('hex')

  const cfg: ProjectConfig = {
    baseUrl,
    jwtSecret,
    githubClientId,
    githubClientSecret,
    googleClientId,
    googleClientSecret,
    emailAuthEnabled,
    smtpHost,
    smtpPort,
    smtpUser,
    smtpPass,
    smtpFrom,
  }

  const dir = join(process.cwd(), projectName)
  await mkdir(join(dir, 'data'), { recursive: true })

  await writeFile(join(dir, 'docker-compose.yml'), dockerCompose(), 'utf8')
  log.success('Created docker-compose.yml')

  await writeFile(join(dir, '.env'), dotEnv(cfg), 'utf8')
  log.success('Created .env')

  await writeFile(join(dir, '.gitignore'), gitignore(), 'utf8')
  log.success('Created .gitignore')

  await writeFile(join(dir, 'data', '.gitkeep'), '', 'utf8')

  note(
    [
      `cd ${projectName}`,
      '',
      'Start Quipthread:',
      '  docker compose up -d',
      '',
      'Then open your BASE_URL to complete setup.',
      '',
      'Keep .env out of version control — it contains your JWT secret.',
    ].join('\n'),
    'Next steps'
  )
}
