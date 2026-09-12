import { cancel, confirm, isCancel, password, text } from '@clack/prompts'
import type { ProjectConfig } from '../templates/project.js'

type SmtpConfig = Pick<
  ProjectConfig,
  'smtpHost' | 'smtpPort' | 'smtpUser' | 'smtpPass' | 'smtpFrom'
>

export async function promptSmtp(emailAuthEnabled: boolean): Promise<SmtpConfig> {
  let smtpHost = ''
  let smtpPort = '587'
  let smtpUser = ''
  let smtpPass = ''
  let smtpFrom = ''

  const wantSmtp =
    emailAuthEnabled ||
    (await confirm({
      message: 'Configure SMTP for email notifications and verification?',
      initialValue: emailAuthEnabled,
    }))
  if (isCancel(wantSmtp)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  if (wantSmtp) {
    const host = await text({
      message: 'SMTP host',
      placeholder: 'smtp.example.com',
      validate: (v) => {
        if (!v.trim()) return 'Required'
      },
    })
    if (isCancel(host)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    smtpHost = host.trim()

    const port = await text({
      message: 'SMTP port',
      placeholder: '587',
      defaultValue: '587',
    })
    if (isCancel(port)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    smtpPort = port.trim()

    const user = await text({
      message: 'SMTP username (leave blank if your relay needs no authentication)',
    })
    if (isCancel(user)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    smtpUser = (user ?? '').trim()

    const pass = await password({
      message: 'SMTP password (leave blank if not required)',
    })
    if (isCancel(pass)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    smtpPass = pass ?? ''

    const from = await text({
      message: 'Sender email address (e.g. noreply@example.com)',
      validate: (v) => {
        if (!v.trim()) return 'Required'
      },
    })
    if (isCancel(from)) {
      cancel('Cancelled.')
      process.exit(0)
    }
    smtpFrom = from.trim()
  }

  return { smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom }
}
