import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react()],
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
    rollupOptions: {
      // Bundle React into the output so host sites don't need it as a peer dep.
      // No external entries here — everything is inlined.
    },
    outDir: 'dist',
    emptyOutDir: true,
  },
})
