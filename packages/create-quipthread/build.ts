import { readFileSync, writeFileSync, mkdirSync } from 'fs'
import { spawnSync } from 'child_process'

mkdirSync('dist', { recursive: true })

const result = spawnSync(
  'bun',
  ['build', 'src/index.ts', '--target=node', '--outfile=dist/index.js'],
  { stdio: 'inherit' }
)

if (result.status !== 0) process.exit(result.status ?? 1)

// Prepend shebang so the bin is directly executable
const outFile = 'dist/index.js'
writeFileSync(outFile, '#!/usr/bin/env node\n' + readFileSync(outFile, 'utf8'))

spawnSync('chmod', ['+x', outFile], { stdio: 'inherit' })

console.log('\nBuilt: dist/index.js')
