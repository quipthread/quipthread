import { access, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { cancel, isCancel, log, note, select } from '@clack/prompts'
import {
  flyioInstructions,
  railwayInstructions,
  railwayToml,
  renderInstructions,
  renderYaml,
} from '../templates/platforms.js'

export async function deployFlow(): Promise<void> {
  try {
    await access(join(process.cwd(), 'Dockerfile'))
  } catch (error) {
    if (error instanceof Error && 'code' in error && error.code === 'ENOENT') {
      cancel('No Dockerfile found. Create a Quipthread project, then run this command inside it.')
      return
    }
    throw error
  }
  const platform = await select({
    message: 'Select a deployment platform',
    options: [
      { value: 'railway', label: 'Railway' },
      { value: 'render', label: 'Render' },
      { value: 'fly', label: 'Fly.io', hint: 'requires flyctl CLI' },
    ],
  })

  if (isCancel(platform)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  switch (platform) {
    case 'railway': {
      await writeFile(join(process.cwd(), 'railway.toml'), railwayToml(), {
        encoding: 'utf8',
        flag: 'wx',
      })
      log.success('Created railway.toml')
      note(railwayInstructions(), 'Deploying to Railway')
      break
    }

    case 'render': {
      await writeFile(join(process.cwd(), 'render.yaml'), renderYaml(), {
        encoding: 'utf8',
        flag: 'wx',
      })
      log.success('Created render.yaml')
      note(renderInstructions(), 'Deploying to Render')
      break
    }

    case 'fly': {
      note(flyioInstructions(), 'Deploying to Fly.io')
      break
    }
  }
}
