import { readFileSync } from 'node:fs'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [
    react(),
    {
      name: 'editorcn-license',
      generateBundle(_options, bundle) {
        const license = readFileSync(new URL('./src/editorcn/LICENSE', import.meta.url), 'utf8')
        for (const output of Object.values(bundle)) {
          if (output.type === 'chunk' && output.isEntry) {
            output.code = `/*! EditorCN portions:\n${license}*/\n${output.code}`
          }
        }
      },
    },
  ],
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
  build: {
    lib: {
      entry: 'src/main.tsx',
      name: 'QuipthreadWidget',
      fileName: 'embed',
      formats: ['iife'],
    },
    rolldownOptions: {
      // Bundle React into the output so host sites don't need it as a peer dep.
      // No external entries here — everything is inlined.
    },
    outDir: 'dist',
    emptyOutDir: true,
  },
})
