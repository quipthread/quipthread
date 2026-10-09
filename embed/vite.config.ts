import { readFileSync } from 'node:fs'
import { defineConfig, minify, type Plugin, transformWithOxc } from 'vite'
import { preactAliases } from './preact-aliases.ts'

const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8')) as {
  version: string
}

// Output layout, served by the backend:
//   embed.js               loader; sites include it, so its name never changes
//   embed/widget-[hash].js and shared chunks; content-hashed, cached forever
const WIDGET = 'widget'

// embed.js is a classic script, so it is compiled on its own rather than as
// a bundle entry: bundling would wrap its import() in Vite's preload helper,
// which uses import.meta. The widget's hashed path is filled in here.
function loader(): Plugin {
  return {
    name: 'quipthread-loader',
    async generateBundle(_options, bundle) {
      const widget = Object.values(bundle).find(
        (output) => output.type === 'chunk' && output.isEntry && output.name === WIDGET,
      )
      if (!widget) throw new Error('widget entry missing from the bundle')
      const path = new URL('./src/loader.ts', import.meta.url)
      const source = readFileSync(path, 'utf8')
      if (!source.includes('__QT_WIDGET_PATH__')) {
        throw new Error('src/loader.ts no longer references __QT_WIDGET_PATH__')
      }
      // Strip types first so only the runtime reference is replaced.
      const { code } = await transformWithOxc(source, path.pathname, { lang: 'ts' })
      const linked = code
        .replaceAll('__QT_WIDGET_PATH__', JSON.stringify(`./${widget.fileName}`))
        // Type-only imports make the source a module; the script needs no marker.
        .replace(/export\s*\{\s*\};?\s*$/, '')
      const minified = await minify('embed.js', linked, { compress: true, mangle: true })
      if (/\bimport\.meta\b|\bimport\s*[{*\w]|\bexport\b/.test(minified.code)) {
        throw new Error('embed.js must stay a classic script: no import.meta, imports or exports')
      }
      this.emitFile({ type: 'asset', fileName: 'embed.js', source: minified.code })
    },
  }
}

// EditorCN is MIT-licensed; keep its notice on the chunk that contains it.
function editorcnLicense(): Plugin {
  return {
    name: 'editorcn-license',
    generateBundle(_options, bundle) {
      const license = readFileSync(new URL('./src/editorcn/LICENSE', import.meta.url), 'utf8')
      for (const output of Object.values(bundle)) {
        if (output.type !== 'chunk') continue
        if (Object.keys(output.modules).some((id) => id.includes('/src/editorcn/'))) {
          output.code = `/*! EditorCN portions:\n${license}*/\n${output.code}`
        }
      }
    },
  }
}

export default defineConfig({
  resolve: {
    alias: preactAliases.map(([specifier, target]) => ({
      find: new RegExp(`^${specifier}$`),
      replacement: target,
    })),
  },
  plugins: [loader(), editorcnLicense()],
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
    __QT_VERSION__: JSON.stringify(pkg.version),
  },
  build: {
    // Dynamic import() and ES modules: every browser the widget supports.
    target: 'es2020',
    modulePreload: false,
    outDir: 'dist',
    emptyOutDir: true,
    rolldownOptions: {
      input: { [WIDGET]: 'src/main.tsx' },
      // Keep start() exported from the widget entry.
      preserveEntrySignatures: 'exports-only',
      output: {
        format: 'es',
        entryFileNames: 'embed/[name]-[hash].js',
        chunkFileNames: 'embed/[name]-[hash].js',
      },
    },
  },
})
