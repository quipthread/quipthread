import { cancel, intro, isCancel, outro, select } from '@clack/prompts'
import { addCommentsFlow } from './flows/add-comments.js'
import { deployFlow } from './flows/deploy.js'
import { newProjectFlow } from './flows/new-project.js'

async function main() {
  const args = process.argv.slice(2)
  if (args.includes('--help') || args.includes('-h')) {
    console.log(
      'Usage: create-quipthread\n\nInteractive setup: add comments, create a Docker project, or prepare platform deployment.\nRun in a new directory for setup. Platform deployment requires a Dockerfile.\nOptions: --help, -h',
    )
    return
  }
  if (args.length > 0) {
    console.error('Unknown option. Run create-quipthread --help for usage.')
    process.exitCode = 1
    return
  }
  console.log()
  intro(' create-quipthread ')

  const action = await select({
    message: 'What would you like to do?',
    options: [
      { value: 'add-comments', label: 'Add comments to my site' },
      { value: 'new-project', label: 'Create a new Quipthread project', hint: 'self-hosted setup' },
      { value: 'deploy', label: 'Deploy to a platform', hint: 'Railway · Render · Fly.io' },
    ],
  })

  if (isCancel(action)) {
    cancel('Cancelled.')
    process.exit(0)
  }

  switch (action) {
    case 'add-comments':
      await addCommentsFlow()
      break
    case 'new-project':
      await newProjectFlow()
      break
    case 'deploy':
      await deployFlow()
      break
  }

  outro('Done!')
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
