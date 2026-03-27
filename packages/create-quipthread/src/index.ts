import { cancel, intro, isCancel, outro, select } from '@clack/prompts'
import { addCommentsFlow } from './flows/add-comments.js'
import { deployFlow } from './flows/deploy.js'
import { newProjectFlow } from './flows/new-project.js'

async function main() {
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
